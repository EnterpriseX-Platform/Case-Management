package app

// Events for other systems. Case changes emit events (case.started,
// case.stage_changed, case.completed, case.cancelled, work.created,
// work.completed, incident.raised). Each event is written to the outbox in
// the same transaction as the change, then delivered by the notify service:
// to webhooks (HMAC signed, retried) and to a message broker when one is
// configured. Nothing is lost if a receiver is down.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/id"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/store"
)

// EventTypes lists the events a webhook can subscribe to.
var EventTypes = []string{"case.started", "case.stage_changed", "case.completed", "case.cancelled",
	"work.created", "work.completed", "incident.raised"}

// Broker publishes events to a message broker (NATS, Kafka ...). Optional.
type Broker interface {
	Publish(ctx context.Context, subject string, payload []byte) error
}

type hookCache struct {
	mu    sync.Mutex
	at    time.Time
	hooks []*store.Webhook
}

func (s *Service) webhooks(ctx context.Context) []*store.Webhook {
	s.hooks.mu.Lock()
	defer s.hooks.mu.Unlock()
	if time.Since(s.hooks.at) > 15*time.Second {
		hs, err := s.S.ListWebhooks(ctx)
		if err == nil {
			s.hooks.hooks, s.hooks.at = hs, time.Now()
		}
	}
	return s.hooks.hooks
}

func (s *Service) flushHooks() {
	s.hooks.mu.Lock()
	s.hooks.at = time.Time{}
	s.hooks.mu.Unlock()
}

// emit queues an event for every matching webhook and the broker. It runs
// inside the caller's transaction. Test cases never emit.
func (s *Service) emit(ctx context.Context, c *store.Case, event string, extra map[string]any) {
	if c == nil || c.Test {
		return
	}
	now := s.Now()
	base := map[string]any{
		"event": event, "at": now.UTC().Format(time.RFC3339Nano), "id": id.New(),
		"case": map[string]any{"id": c.ID, "number": c.Number, "processKey": c.ProcessKey, "title": c.Title,
			"status": c.Status, "stage": c.Stage, "fiscalYear": c.FiscalYear},
	}
	for k, v := range extra {
		base[k] = v
	}
	payload := func(withData bool) string {
		m := base
		if withData && c.Instance != nil {
			m = map[string]any{}
			for k, v := range base {
				m[k] = v
			}
			m["data"] = c.Instance.Vars
		}
		b, _ := json.Marshal(m)
		return string(b)
	}
	for _, h := range s.webhooks(ctx) {
		if !h.Active || !matches(h.Events, event) || (len(h.ProcessKeys) > 0 && !contains(h.ProcessKeys, c.ProcessKey)) {
			continue
		}
		_ = s.S.PutOutbox(ctx, &store.OutboxMsg{ID: id.New(), Kind: "webhook", Hook: h.ID, To: h.URL, Subject: event,
			Body: payload(h.IncludeData), Status: "pending", NextRunAt: now, CreatedAt: now})
	}
	if s.Broker != nil {
		_ = s.S.PutOutbox(ctx, &store.OutboxMsg{ID: id.New(), Kind: "event", To: "cm." + event, Subject: event,
			Body: payload(true), Status: "pending", NextRunAt: now, CreatedAt: now})
	}
}

func matches(events []string, e string) bool {
	for _, x := range events {
		if x == "*" || x == e || (strings.HasSuffix(x, ".*") && strings.HasPrefix(e, strings.TrimSuffix(x, "*"))) {
			return true
		}
	}
	return false
}

// HookClient is the HTTP client for webhook delivery (tests replace it).
var HookClient = &http.Client{Timeout: 10 * time.Second}

// deliverWebhook posts one event. Receivers verify
//
//	X-CM-Signature: sha256=hex(HMAC_SHA256(secret, X-CM-Timestamp + "." + body))
//
// and use X-CM-Delivery to drop duplicates (a delivery may be retried).
func (s *Service) deliverWebhook(ctx context.Context, m *store.OutboxMsg) error {
	var hook *store.Webhook
	for _, h := range s.webhooks(ctx) {
		if h.ID == m.Hook {
			hook = h
		}
	}
	if hook == nil || !hook.Active {
		return nil // removed or paused: drop
	}
	u, err := url.Parse(m.To)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return fmt.Errorf("webhook %s: bad URL", hook.Name)
	}
	if allow := os.Getenv("CM_CONNECTOR_ALLOW"); allow != "" && !hostAllowed(u.Hostname(), allow) {
		return fmt.Errorf("webhook %s: host %s is not in CM_CONNECTOR_ALLOW", hook.Name, u.Hostname())
	}
	ts := strconv.FormatInt(s.Now().Unix(), 10)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.To, bytes.NewBufferString(m.Body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "case-management-webhook/1")
	propagate(ctx, req)
	req.Header.Set("X-CM-Event", m.Subject)
	req.Header.Set("X-CM-Delivery", m.ID)
	req.Header.Set("X-CM-Timestamp", ts)
	if hook.Secret != "" {
		req.Header.Set("X-CM-Signature", "sha256="+Sign(SecretValue(hook.Secret), ts, []byte(m.Body)))
	}
	res, err := HookClient.Do(req)
	now := s.Now()
	hook.LastAt = &now
	if err != nil {
		hook.LastStatus, hook.LastError = "error", err.Error()
	} else {
		res.Body.Close()
		hook.LastStatus = strconv.Itoa(res.StatusCode)
		if res.StatusCode >= 300 {
			hook.LastError = res.Status
			err = fmt.Errorf("webhook %s: %s", hook.Name, res.Status)
		} else {
			hook.LastError = ""
		}
	}
	_ = s.S.PutWebhook(ctx, hook)
	return err
}

// Sign returns the hex HMAC-SHA256 of timestamp + "." + body.
func Sign(secret, ts string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature checks an inbound signature and rejects old timestamps
// (replays older than 5 minutes).
func VerifySignature(secret, ts, sig string, body []byte, now time.Time) bool {
	if secret == "" || ts == "" {
		return false
	}
	n, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || now.Sub(time.Unix(n, 0)).Abs() > 5*time.Minute {
		return false
	}
	want := "sha256=" + Sign(secret, ts, body)
	return hmac.Equal([]byte(want), []byte(sig))
}

// SecretValue reads a named secret from CM_SECRET_<NAME>.
func SecretValue(name string) string {
	return os.Getenv("CM_SECRET_" + strings.ToUpper(strings.NewReplacer("-", "_", ".", "_").Replace(name)))
}

// ---- webhook admin

func (s *Service) Webhooks(ctx context.Context, by Actor) ([]*store.Webhook, error) {
	if !by.Has(RoleAdmin) {
		return nil, ErrForbidden
	}
	hs, err := s.S.ListWebhooks(ctx)
	if hs == nil {
		hs = []*store.Webhook{}
	}
	return hs, err
}

func (s *Service) SaveWebhook(ctx context.Context, by Actor, h *store.Webhook) (*store.Webhook, error) {
	if !by.Has(RoleAdmin) {
		return nil, ErrForbidden
	}
	u, err := url.Parse(h.URL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, invalid("the URL must start with https:// or http://")
	}
	if strings.TrimSpace(h.Name) == "" {
		return nil, invalid("a webhook needs a name")
	}
	if len(h.Events) == 0 {
		return nil, invalid("choose at least one event")
	}
	for _, e := range h.Events {
		if e != "*" && !contains(EventTypes, e) && !strings.HasSuffix(e, ".*") {
			return nil, invalid("unknown event %q", e)
		}
	}
	if h.Secret != "" && !keyPattern.MatchString(strings.ToLower(strings.ReplaceAll(h.Secret, "_", "-"))) {
		return nil, invalid("secret must be a name such as erp_webhook (its value is set as CM_SECRET_ERP_WEBHOOK)")
	}
	if h.ID == "" {
		h.ID = id.New()
	}
	h.UpdatedAt = s.Now()
	if err := s.S.PutWebhook(ctx, h); err != nil {
		return nil, err
	}
	s.flushHooks()
	s.audit(ctx, by, "webhook.save", "webhook", h.ID, map[string]any{"name": h.Name, "url": h.URL, "events": h.Events})
	return h, nil
}

func (s *Service) DeleteWebhook(ctx context.Context, by Actor, hookID string) error {
	if !by.Has(RoleAdmin) {
		return ErrForbidden
	}
	s.flushHooks()
	s.audit(ctx, by, "webhook.delete", "webhook", hookID, nil)
	return s.S.DeleteWebhook(ctx, hookID)
}

// TestWebhook queues a test event for one webhook.
func (s *Service) TestWebhook(ctx context.Context, by Actor, hookID string) error {
	if !by.Has(RoleAdmin) {
		return ErrForbidden
	}
	now := s.Now()
	body, _ := json.Marshal(map[string]any{"event": "webhook.test", "at": now.UTC().Format(time.RFC3339), "id": id.New()})
	return s.S.PutOutbox(ctx, &store.OutboxMsg{ID: id.New(), Kind: "webhook", Hook: hookID, Subject: "webhook.test",
		To: func() string {
			for _, h := range s.webhooks(ctx) {
				if h.ID == hookID {
					return h.URL
				}
			}
			return ""
		}(), Body: string(body), Status: "pending", NextRunAt: now, CreatedAt: now})
}

// Deliveries lists messages that are still waiting or gave up.
func (s *Service) Deliveries(ctx context.Context, by Actor, status string) ([]*store.OutboxMsg, error) {
	if !by.Has(RoleAdmin) {
		return nil, ErrForbidden
	}
	ms, err := s.S.ListOutbox(ctx, "", status, 200)
	if ms == nil {
		ms = []*store.OutboxMsg{}
	}
	return ms, err
}

// ---- extension steps

// ActionFunc is a step implemented in Go and compiled into the server.
// A service task with cm:action="name" calls it; its result is merged into
// the case data. Inputs are the task's cm:* attributes with ${field}
// placeholders already filled in.
type ActionFunc func(ctx context.Context, s *Service, c *store.Case, in map[string]string) (map[string]any, error)

var (
	actionsMu sync.RWMutex
	actions   = map[string]ActionFunc{}
)

// RegisterAction adds an extension step. Call it from an init function in a
// package that is imported by cmd/server (see docs/EXTENDING.md).
func RegisterAction(name string, fn ActionFunc) {
	actionsMu.Lock()
	defer actionsMu.Unlock()
	actions[name] = fn
}

// Actions lists registered extension steps (for the studio).
func Actions() []string {
	actionsMu.RLock()
	defer actionsMu.RUnlock()
	out := make([]string, 0, len(actions))
	for k := range actions {
		out = append(out, k)
	}
	return out
}

func action(name string) ActionFunc {
	actionsMu.RLock()
	defer actionsMu.RUnlock()
	return actions[name]
}

func init() {
	// email.send: cm:to, cm:subject, cm:body (all may use ${field}).
	RegisterAction("email.send", func(ctx context.Context, s *Service, c *store.Case, in map[string]string) (map[string]any, error) {
		to := strings.TrimSpace(in["to"])
		if to == "" {
			return nil, fmt.Errorf("email.send: no recipient")
		}
		now := s.Now()
		for _, addr := range strings.FieldsFunc(to, func(r rune) bool { return r == ',' || r == ';' || r == ' ' }) {
			if !strings.Contains(addr, "@") {
				return nil, fmt.Errorf("email.send: %q is not an e-mail address", addr)
			}
			if err := s.S.PutOutbox(ctx, &store.OutboxMsg{ID: id.New(), Kind: "email", To: addr, Subject: in["subject"], Body: in["body"],
				Status: "pending", NextRunAt: now, CreatedAt: now}); err != nil {
				return nil, err
			}
		}
		return map[string]any{"emailQueuedAt": now.UTC().Format(time.RFC3339)}, nil
	})
}
