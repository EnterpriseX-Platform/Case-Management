package api

// Request tracing and limits.
//
//   - Every request gets a W3C trace context: an incoming `traceparent`
//     header is kept (so a gateway or the calling system can follow the
//     request), otherwise a new one is made. It is returned in the response,
//     added to every log line of the request and passed on to connector and
//     webhook calls (internal/app reads it from the context).
//   - LOG_FORMAT=json switches logs to one JSON object per line.
//   - RATE_LIMIT (requests per second per user, default 30; 0 = off) with a
//     burst of twice that; over the limit the API answers 429 Retry-After.
//     Limits are per replica.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"math"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/app"
)

// ---- trace context

func newTraceID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func newSpanID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// traceparent returns the trace id and a traceparent for this hop.
func traceparent(in string) (traceID, header string) {
	p := strings.Split(strings.TrimSpace(in), "-")
	if len(p) == 4 && len(p[1]) == 32 && len(p[2]) == 16 && strings.Trim(p[1], "0") != "" {
		if _, err := hex.DecodeString(p[1]); err == nil {
			return p[1], "00-" + p[1] + "-" + newSpanID() + "-" + p[3]
		}
	}
	t := newTraceID()
	return t, "00-" + t + "-" + newSpanID() + "-01"
}

// SetupLogging picks text or JSON logs and adds the trace id to log lines.
func SetupLogging() {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	if os.Getenv("LOG_LEVEL") == "debug" {
		opts.Level = slog.LevelDebug
	}
	var h slog.Handler = slog.NewTextHandler(os.Stderr, opts)
	if os.Getenv("LOG_FORMAT") == "json" {
		h = slog.NewJSONHandler(os.Stderr, opts)
	}
	slog.SetDefault(slog.New(traceHandler{h}))
}

type traceHandler struct{ slog.Handler }

func (t traceHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := app.TraceID(ctx); id != "" {
		r.AddAttrs(slog.String("trace_id", id))
	}
	return t.Handler.Handle(ctx, r)
}
func (t traceHandler) WithAttrs(a []slog.Attr) slog.Handler {
	return traceHandler{t.Handler.WithAttrs(a)}
}
func (t traceHandler) WithGroup(n string) slog.Handler { return traceHandler{t.Handler.WithGroup(n)} }

// ---- rate limit (token bucket per user or client address)

type bucket struct {
	tokens float64
	last   time.Time
}

type limiter struct {
	mu      sync.Mutex
	rate    float64
	burst   float64
	buckets map[string]*bucket
	seen    time.Time
}

func newLimiter() *limiter {
	rate := 30.0
	if v := os.Getenv("RATE_LIMIT"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			rate = f
		}
	}
	return &limiter{rate: rate, burst: math.Max(rate*2, 1), buckets: map[string]*bucket{}}
}

// allow reports whether key may make a request now, and if not, how long to wait.
func (l *limiter) allow(key string, now time.Time) (bool, time.Duration) {
	if l.rate <= 0 {
		return true, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.seen) > time.Minute { // forget idle clients
		for k, b := range l.buckets {
			if now.Sub(b.last) > 5*time.Minute {
				delete(l.buckets, k)
			}
		}
		l.seen = now
	}
	b := l.buckets[key]
	if b == nil {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	b.tokens = math.Min(l.burst, b.tokens+now.Sub(b.last).Seconds()*l.rate)
	b.last = now
	if b.tokens < 1 {
		return false, time.Duration((1 - b.tokens) / l.rate * float64(time.Second))
	}
	b.tokens--
	return true, 0
}

func clientKey(r *http.Request, user string) string {
	if user != "" {
		return "u:" + user
	}
	if f := r.Header.Get("X-Forwarded-For"); f != "" {
		return "ip:" + strings.TrimSpace(strings.Split(f, ",")[0])
	}
	h, _, _ := net.SplitHostPort(r.RemoteAddr)
	return "ip:" + h
}
