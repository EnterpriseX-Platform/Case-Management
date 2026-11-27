package api

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/app"
)

// OIDC validates bearer access tokens issued by any OpenID Connect provider
// (Keycloak, Entra ID, Okta, an in-house IAM ...). Only the standard library
// is used: discovery + JWKS + RS256/ES256 signature check.
type OIDC struct {
	Issuer     string
	Audience   string // optional: required "aud"
	RolesClaim string // dotted path, e.g. "roles", "realm_access.roles", "groups"
	RolePrefix string // optional mapping: only claims with this prefix, prefix stripped
	Client     *http.Client

	mu      sync.Mutex
	keys    map[string]crypto.PublicKey
	fetched time.Time
}

func NewOIDC(issuer, audience, rolesClaim string) *OIDC {
	if rolesClaim == "" {
		rolesClaim = "roles"
	}
	return &OIDC{Issuer: strings.TrimRight(issuer, "/"), Audience: audience, RolesClaim: rolesClaim,
		Client: &http.Client{Timeout: 10 * time.Second}}
}

// errUnauthorized means no valid identity (401); app.ErrForbidden means the
// identity lacks a role (403).
var errUnauthorized = errors.New("missing or invalid token")

// Auth implements Server.Auth.
func (o *OIDC) Auth(r *http.Request) (app.Actor, error) {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return app.Actor{}, errUnauthorized
	}
	claims, err := o.Verify(r.Context(), strings.TrimPrefix(h, "Bearer "))
	if err != nil {
		return app.Actor{}, fmt.Errorf("%w: %v", app.ErrForbidden, err)
	}
	a := app.Actor{ID: str(claims["preferred_username"]), Name: str(claims["name"])}
	if a.ID == "" {
		a.ID = str(claims["sub"])
	}
	if a.Name == "" {
		a.Name = a.ID
	}
	for _, role := range o.roles(claims) {
		if o.RolePrefix != "" {
			if !strings.HasPrefix(role, o.RolePrefix) {
				continue
			}
			role = strings.TrimPrefix(role, o.RolePrefix)
		}
		a.Roles = append(a.Roles, role)
	}
	return a, nil
}

func (o *OIDC) roles(claims map[string]any) []string {
	var cur any = claims
	for _, part := range strings.Split(o.RolesClaim, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[part]
	}
	var out []string
	switch v := cur.(type) {
	case []any:
		for _, x := range v {
			out = append(out, strings.TrimPrefix(fmt.Sprint(x), "/"))
		}
	case string:
		out = strings.Fields(strings.ReplaceAll(v, ",", " "))
	}
	return out
}

// Verify checks signature, issuer, audience and expiry; returns the claims.
func (o *OIDC) Verify(ctx context.Context, token string) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed token")
	}
	var hdr struct{ Alg, Kid string }
	if err := decodeSeg(parts[0], &hdr); err != nil {
		return nil, err
	}
	key, err := o.key(ctx, hdr.Kid)
	if err != nil {
		return nil, err
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, err
	}
	signed := []byte(parts[0] + "." + parts[1])
	switch hdr.Alg {
	case "RS256", "RS512":
		k, ok := key.(*rsa.PublicKey)
		if !ok {
			return nil, errors.New("key type mismatch")
		}
		h, sum := crypto.SHA256, sha256.Sum256(signed)
		digest := sum[:]
		if hdr.Alg == "RS512" {
			s := sha512.Sum512(signed)
			h, digest = crypto.SHA512, s[:]
		}
		if err := rsa.VerifyPKCS1v15(k, h, digest, sig); err != nil {
			return nil, errors.New("bad signature")
		}
	case "ES256":
		k, ok := key.(*ecdsa.PublicKey)
		if !ok || len(sig) != 64 {
			return nil, errors.New("key type mismatch")
		}
		sum := sha256.Sum256(signed)
		if !ecdsa.Verify(k, sum[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
			return nil, errors.New("bad signature")
		}
	default:
		return nil, fmt.Errorf("unsupported alg %q", hdr.Alg)
	}
	var claims map[string]any
	if err := decodeSeg(parts[1], &claims); err != nil {
		return nil, err
	}
	if str(claims["iss"]) != o.Issuer {
		return nil, errors.New("wrong issuer")
	}
	now := float64(time.Now().Unix())
	if exp, ok := claims["exp"].(float64); !ok || now > exp+30 {
		return nil, errors.New("token expired")
	}
	if nbf, ok := claims["nbf"].(float64); ok && now+30 < nbf {
		return nil, errors.New("token not yet valid")
	}
	if o.Audience != "" && !hasAud(claims["aud"], o.Audience) && str(claims["azp"]) != o.Audience {
		return nil, errors.New("wrong audience")
	}
	return claims, nil
}

func hasAud(v any, want string) bool {
	switch a := v.(type) {
	case string:
		return a == want
	case []any:
		for _, x := range a {
			if fmt.Sprint(x) == want {
				return true
			}
		}
	}
	return false
}

// key returns the signing key, refreshing the JWKS on unknown kid (rotation)
// at most once a minute.
func (o *OIDC) key(ctx context.Context, kid string) (crypto.PublicKey, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if k, ok := o.keys[kid]; ok {
		return k, nil
	}
	if time.Since(o.fetched) < time.Minute && o.keys != nil {
		return nil, errors.New("unknown key id")
	}
	if err := o.fetch(ctx); err != nil {
		return nil, err
	}
	if k, ok := o.keys[kid]; ok {
		return k, nil
	}
	if kid == "" && len(o.keys) == 1 {
		for _, k := range o.keys {
			return k, nil
		}
	}
	return nil, errors.New("unknown key id")
}

func (o *OIDC) fetch(ctx context.Context) error {
	var disc struct {
		JWKS string `json:"jwks_uri"`
	}
	if err := o.getJSON(ctx, o.Issuer+"/.well-known/openid-configuration", &disc); err != nil {
		return fmt.Errorf("oidc discovery: %w", err)
	}
	var set struct {
		Keys []struct {
			Kid, Kty, Use, N, E, Crv, X, Y string
		} `json:"keys"`
	}
	if err := o.getJSON(ctx, disc.JWKS, &set); err != nil {
		return fmt.Errorf("oidc jwks: %w", err)
	}
	keys := map[string]crypto.PublicKey{}
	for _, k := range set.Keys {
		if k.Use != "" && k.Use != "sig" {
			continue
		}
		switch k.Kty {
		case "RSA":
			n, err1 := base64.RawURLEncoding.DecodeString(k.N)
			e, err2 := base64.RawURLEncoding.DecodeString(k.E)
			if err1 != nil || err2 != nil {
				continue
			}
			keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
		case "EC":
			if k.Crv != "P-256" {
				continue
			}
			x, err1 := base64.RawURLEncoding.DecodeString(k.X)
			y, err2 := base64.RawURLEncoding.DecodeString(k.Y)
			if err1 != nil || err2 != nil {
				continue
			}
			keys[k.Kid] = &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}
		}
	}
	o.keys, o.fetched = keys, time.Now()
	return nil
}

func (o *OIDC) getJSON(ctx context.Context, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := o.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

func decodeSeg(seg string, v any) error {
	b, err := base64.RawURLEncoding.DecodeString(seg)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func str(v any) string { s, _ := v.(string); return s }
