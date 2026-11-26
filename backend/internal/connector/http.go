// Package connector implements engine.Handlers over HTTP.
//
// serviceTask / sendTask:  cm:url="https://..." cm:method="POST" (body = case variables,
//
//	JSON response object is merged back into the case)
//
// businessRuleTask:        cm:rule="<rule id>" -> POST {RULES_URL}/{rule}/execute
//
//	(any rule engine exposing an execute API)
//
// A response {"bpmnError":"CODE"} raises a catchable BPMN error.
package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/bpmn"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/engine"
)

type HTTP struct {
	RulesURL   string
	RulesToken string
	Client     *http.Client
}

func New(rulesURL, token string) *HTTP {
	return &HTTP{RulesURL: strings.TrimRight(rulesURL, "/"), RulesToken: token, Client: &http.Client{Timeout: 30 * time.Second}}
}

func (h *HTTP) Service(ctx context.Context, n *bpmn.Node, vars map[string]any) (map[string]any, error) {
	url := n.Ext["url"]
	if url == "" {
		return nil, nil // no connector configured: pass-through step
	}
	method := strings.ToUpper(n.Ext["method"])
	if method == "" {
		method = http.MethodPost
	}
	return h.call(ctx, method, url, "", vars)
}

func (h *HTTP) Rule(ctx context.Context, n *bpmn.Node, vars map[string]any) (map[string]any, error) {
	rule := n.Ext["rule"]
	if rule == "" || h.RulesURL == "" {
		return nil, nil
	}
	out, err := h.call(ctx, http.MethodPost, h.RulesURL+"/"+rule+"/execute", h.RulesToken, map[string]any{"input": vars})
	if err != nil {
		return nil, err
	}
	if res, ok := out["output"].(map[string]any); ok {
		return res, nil
	}
	return out, nil
}

// KeyFunc, set by the application, returns the idempotency key of the job
// being executed; it is sent so remote systems can drop duplicate calls.
var KeyFunc func(context.Context) string

func (h *HTTP) call(ctx context.Context, method, url, token string, body any) (map[string]any, error) {
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if KeyFunc != nil {
		if k := KeyFunc(ctx); k != "" {
			req.Header.Set("Idempotency-Key", k)
		}
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := h.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connector %s: %w", url, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	if code, ok := out["bpmnError"].(string); ok && code != "" {
		return nil, &engine.BPMNError{Code: code, Message: fmt.Sprint(out["message"])}
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("connector %s: HTTP %d", url, resp.StatusCode)
	}
	return out, nil
}
