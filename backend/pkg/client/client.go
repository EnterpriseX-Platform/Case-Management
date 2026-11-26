// Package client is the Go SDK for the Case Management API: start cases,
// complete work, send messages and signals from other systems.
//
//	c := client.New("https://cases.example.com", client.Bearer(token))
//	cs, err := c.StartCase(ctx, client.StartInput{ProcessKey: "purchase-request", Vars: map[string]any{"amount": 1000}})
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	Base string
	HTTP *http.Client
	auth func(*http.Request)
}

type Option func(*Client)

// Bearer authenticates with an OIDC access token.
func Bearer(token string) Option {
	return func(c *Client) { c.auth = func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) } }
}

// DevUser uses development headers (servers without OIDC only).
func DevUser(user string, roles ...string) Option {
	return func(c *Client) {
		c.auth = func(r *http.Request) { r.Header.Set("X-User", user); r.Header.Set("X-Roles", strings.Join(roles, ",")) }
	}
}

func New(base string, opts ...Option) *Client {
	c := &Client{Base: strings.TrimRight(base, "/"), HTTP: &http.Client{Timeout: 30 * time.Second}}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Error is an API error with its HTTP status.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return fmt.Sprintf("case management API %d: %s", e.Status, e.Message) }

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.Base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.auth != nil {
		c.auth(req)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		var e struct{ Error string }
		_ = json.Unmarshal(raw, &e)
		return &Error{Status: resp.StatusCode, Message: e.Error}
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

type StartInput struct {
	ProcessKey string         `json:"processKey"`
	FiscalYear int            `json:"fiscalYear,omitempty"`
	Title      string         `json:"title,omitempty"`
	Vars       map[string]any `json:"vars,omitempty"`
	ParentID   string         `json:"parentId,omitempty"`
}

type Case struct {
	ID         string `json:"id"`
	Number     string `json:"number"`
	ProcessKey string `json:"processKey"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	Stage      string `json:"stage"`
	Instance   struct {
		Vars map[string]any `json:"vars"`
	} `json:"instance"`
}

type Work struct {
	ID         string     `json:"id"`
	CaseID     string     `json:"caseId"`
	CaseNumber string     `json:"caseNumber"`
	Node       string     `json:"node"`
	Name       string     `json:"name"`
	Role       string     `json:"role"`
	Assignee   string     `json:"assignee"`
	Status     string     `json:"status"`
	DueAt      *time.Time `json:"dueAt"`
}

func (c *Client) StartCase(ctx context.Context, in StartInput) (*Case, error) {
	var out Case
	return &out, c.do(ctx, http.MethodPost, "/api/cases", in, &out)
}

func (c *Client) GetCase(ctx context.Context, id string) (*Case, error) {
	var out struct {
		Case Case `json:"case"`
	}
	err := c.do(ctx, http.MethodGet, "/api/cases/"+id, nil, &out)
	return &out.Case, err
}

func (c *Client) MyWork(ctx context.Context) ([]Work, error) {
	var out []Work
	return out, c.do(ctx, http.MethodGet, "/api/work", nil, &out)
}

func (c *Client) CompleteWork(ctx context.Context, workID string, data map[string]any) (*Case, error) {
	var out Case
	return &out, c.do(ctx, http.MethodPost, "/api/work/"+workID+"/complete", data, &out)
}

func (c *Client) RunAction(ctx context.Context, caseID, action string, data map[string]any) error {
	return c.do(ctx, http.MethodPost, "/api/cases/"+caseID+"/actions/"+action, data, nil)
}

// Message correlates a message to one case (caseID) or by correlation key.
func (c *Client) Message(ctx context.Context, name, caseID, correlationKey string, vars map[string]any) (int, error) {
	var out struct{ Matched int }
	err := c.do(ctx, http.MethodPost, "/api/messages", map[string]any{"name": name, "caseId": caseID, "correlationKey": correlationKey, "vars": vars}, &out)
	return out.Matched, err
}

func (c *Client) Signal(ctx context.Context, name string, vars map[string]any) error {
	return c.do(ctx, http.MethodPost, "/api/signals", map[string]any{"name": name, "vars": vars}, nil)
}
