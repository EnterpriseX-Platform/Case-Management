package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/app"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/store"
)

func TestTraceparent(t *testing.T) {
	in := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	id, h := traceparent(in)
	if id != "4bf92f3577b34da6a3ce929d0e0e4736" || !strings.HasPrefix(h, "00-"+id+"-") || h == in {
		t.Fatalf("kept trace wrong: %s %s", id, h)
	}
	for _, bad := range []string{"", "garbage", "00-00000000000000000000000000000000-00f067aa0ba902b7-01"} {
		id, h := traceparent(bad)
		if len(id) != 32 || !strings.HasPrefix(h, "00-"+id) {
			t.Fatalf("new trace for %q: %s %s", bad, id, h)
		}
	}
}

func TestLimiter(t *testing.T) {
	l := &limiter{rate: 2, burst: 4, buckets: map[string]*bucket{}}
	now := time.Now()
	for i := 0; i < 4; i++ {
		if ok, _ := l.allow("u:a", now); !ok {
			t.Fatalf("burst refused at %d", i)
		}
	}
	if ok, wait := l.allow("u:a", now); ok || wait <= 0 {
		t.Fatal("over the burst was allowed")
	}
	if ok, _ := l.allow("u:b", now); !ok {
		t.Fatal("another user was limited")
	}
	if ok, _ := l.allow("u:a", now.Add(600*time.Millisecond)); !ok {
		t.Fatal("tokens did not refill")
	}
}

func TestMiddlewareTraceAndLimit(t *testing.T) {
	t.Setenv("RATE_LIMIT", "1")
	srv := (&Server{S: app.New(store.NewMemory(), nil, []byte("x"))}).Handler()
	call := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/api/v1/processes", nil)
		r.Header.Set("X-User", "u1")
		r.Header.Set("X-Roles", "cm:designer")
		r.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		return w
	}
	first := call()
	if first.Code != http.StatusOK {
		t.Fatalf("/api/v1 path: %d %s", first.Code, first.Body.String())
	}
	if first.Header().Get("X-Request-ID") != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("trace not kept: %q", first.Header().Get("X-Request-ID"))
	}
	call() // burst of 2
	if w := call(); w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") == "" {
		t.Fatalf("limit not applied: %d", w.Code)
	}
}
