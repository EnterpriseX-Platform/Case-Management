package api

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func seg(v any) string { b, _ := json.Marshal(v); return base64.RawURLEncoding.EncodeToString(b) }

func TestOIDCVerify(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			json.NewEncoder(w).Encode(map[string]string{"jwks_uri": srv.URL + "/jwks"})
		case "/jwks":
			json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
				"kid": "k1", "kty": "RSA", "use": "sig",
				"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
				"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
			}}})
		}
	}))
	defer srv.Close()

	sign := func(claims map[string]any) string {
		head := seg(map[string]string{"alg": "RS256", "kid": "k1", "typ": "JWT"})
		body := seg(claims)
		sum := sha256.Sum256([]byte(head + "." + body))
		sig, _ := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
		return head + "." + body + "." + base64.RawURLEncoding.EncodeToString(sig)
	}
	o := NewOIDC(srv.URL, "case-web", "realm_access.roles")
	good := map[string]any{"iss": srv.URL, "aud": "case-web", "exp": time.Now().Add(time.Hour).Unix(),
		"preferred_username": "somchai", "realm_access": map[string]any{"roles": []string{"cm:supervisor", "approver"}}}

	r := httptest.NewRequest("GET", "/api/me", nil)
	r.Header.Set("Authorization", "Bearer "+sign(good))
	a, err := o.Auth(r)
	if err != nil || a.ID != "somchai" || len(a.Roles) != 2 || !a.Has("cm:supervisor") {
		t.Fatalf("auth: %+v %v", a, err)
	}

	bad := []map[string]any{}
	for _, mut := range []func(m map[string]any){
		func(m map[string]any) { m["iss"] = "https://evil" },
		func(m map[string]any) { m["aud"] = "other" },
		func(m map[string]any) { m["exp"] = time.Now().Add(-time.Hour).Unix() },
	} {
		m := map[string]any{}
		for k, v := range good {
			m[k] = v
		}
		mut(m)
		bad = append(bad, m)
	}
	for i, c := range bad {
		r.Header.Set("Authorization", "Bearer "+sign(c))
		if _, err := o.Auth(r); err == nil {
			t.Fatalf("case %d accepted", i)
		}
	}
	// tampered payload
	tok := sign(good)
	r.Header.Set("Authorization", "Bearer "+tok[:len(tok)-4]+"AAAA")
	if _, err := o.Auth(r); err == nil {
		t.Fatal("tampered token accepted")
	}
	// header auth must not work when OIDC is on
	r2 := httptest.NewRequest("GET", "/api/me", nil)
	r2.Header.Set("X-User", "admin")
	r2.Header.Set("X-Roles", "cm:admin")
	if _, err := o.Auth(r2); err == nil {
		t.Fatal("dev headers accepted under OIDC")
	}
}
