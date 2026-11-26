package app

// Studio runtime: decision tables and REST connectors designed in the App
// Studio are executed by the job runner (no external rule engine needed).

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/bpmn"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/engine"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/store"
)

// EvalDecision evaluates a decision table against case data.
func EvalDecision(dt *store.DecisionTable, vars map[string]any) (map[string]any, []int, error) {
	out := map[string]any{}
	var hits []int
	for ri, row := range dt.Rows {
		match := true
		for ci, in := range dt.Inputs {
			cell := ""
			if ci < len(row.When) {
				cell = strings.TrimSpace(row.When[ci])
			}
			if cell == "" || cell == "-" || cell == "*" {
				continue
			}
			ok, err := engine.EvalBool(cellExpr(in.Key, cell), vars)
			if err != nil {
				return nil, nil, fmt.Errorf("row %d, %s: %w", ri+1, in.Label, err)
			}
			if !ok {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		hits = append(hits, ri)
		for ci, o := range dt.Outputs {
			if ci >= len(row.Then) || strings.TrimSpace(row.Then[ci]) == "" {
				continue
			}
			v, err := engine.EvalAny(row.Then[ci], vars)
			if err != nil {
				v = row.Then[ci] // plain text literal
			}
			if dt.Hit == "collect" {
				l, _ := out[o.Key].([]any)
				out[o.Key] = append(l, v)
			} else {
				out[o.Key] = v
			}
		}
		if dt.Hit != "collect" {
			break
		}
	}
	return out, hits, nil
}

var rangeCell = regexp.MustCompile(`^(-?[\d.]+)\s*\.\.\s*(-?[\d.]+)$`)

// cellExpr turns a table cell into a boolean expression over the input.
func cellExpr(input, cell string) string {
	if m := rangeCell.FindStringSubmatch(cell); m != nil {
		return fmt.Sprintf("(%s) >= %s && (%s) <= %s", input, m[1], input, m[2])
	}
	for _, op := range []string{">=", "<=", "!=", "==", ">", "<", "in ", "not in ", "matches ", "contains ", "startsWith "} {
		if strings.HasPrefix(cell, op) {
			return "(" + input + ") " + cell
		}
	}
	if cell == "true" || cell == "false" {
		return "(" + input + ") == " + cell
	}
	if _, err := strconv.ParseFloat(cell, 64); err == nil {
		return "(" + input + ") == " + cell
	}
	if strings.HasPrefix(cell, `"`) {
		return "(" + input + ") == " + cell
	}
	return "(" + input + ") == " + strconv.Quote(cell)
}

var tmpl = regexp.MustCompile(`\$\{([^}]+)\}`)

func expand(s string, vars map[string]any) string {
	return tmpl.ReplaceAllStringFunc(s, func(m string) string {
		k := strings.TrimSpace(m[2 : len(m)-1])
		if strings.HasPrefix(k, "secret:") {
			// secrets never live in the model: CM_SECRET_<NAME> environment variables
			return os.Getenv("CM_SECRET_" + strings.ToUpper(strings.TrimPrefix(k, "secret:")))
		}
		if v, err := engine.EvalAny(k, vars); err == nil && v != nil {
			return fmt.Sprint(v)
		}
		return ""
	})
}

// ConnectorResult is returned by a connector test call.
type ConnectorResult struct {
	Status   int            `json:"status"`
	Request  map[string]any `json:"request"`
	Response any            `json:"response"`
	Output   map[string]any `json:"output"`
	Millis   int64          `json:"millis"`
	Error    string         `json:"error,omitempty"`
}

// CallConnector executes a studio connector with case data.
func CallConnector(ctx context.Context, c *store.Connector, vars map[string]any, idemKey string) (*ConnectorResult, error) {
	res := &ConnectorResult{Request: map[string]any{}, Output: map[string]any{}}
	url := expand(c.URL, vars)
	method := strings.ToUpper(firstNonEmpty(c.Method, "POST"))
	var body io.Reader
	reqBody := map[string]any{}
	for k, ex := range c.Body {
		v, err := engine.EvalAny(ex, vars)
		if err != nil {
			v = expand(ex, vars)
		}
		reqBody[k] = v
	}
	if method != http.MethodGet && method != http.MethodDelete {
		b, _ := json.Marshal(reqBody)
		body = bytes.NewReader(b)
	}
	res.Request = map[string]any{"method": method, "url": url, "body": reqBody}
	timeout := time.Duration(c.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, method, url, body)
	if err != nil {
		return res, err
	}
	if allow := os.Getenv("CM_CONNECTOR_ALLOW"); allow != "" && !hostAllowed(req.URL.Hostname(), allow) {
		return res, fmt.Errorf("connector %s: host %s is not in CM_CONNECTOR_ALLOW", c.Name, req.URL.Hostname())
	}
	req.Header.Set("Content-Type", "application/json")
	if idemKey != "" {
		req.Header.Set("Idempotency-Key", idemKey)
	}
	for k, v := range c.Headers {
		req.Header.Set(k, expand(v, vars))
	}
	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	res.Millis = time.Since(start).Milliseconds()
	if err != nil {
		return res, fmt.Errorf("connector %s: %w", c.Name, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	res.Status = resp.StatusCode
	var parsed any
	if json.Unmarshal(raw, &parsed) != nil {
		parsed = string(raw)
	}
	res.Response = parsed
	if m, ok := parsed.(map[string]any); ok {
		if code, ok := m["bpmnError"].(string); ok && code != "" {
			return res, &engine.BPMNError{Code: code, Message: fmt.Sprint(m["message"])}
		}
	}
	if resp.StatusCode >= 300 {
		return res, fmt.Errorf("connector %s: HTTP %d", c.Name, resp.StatusCode)
	}
	for field, path := range c.Response {
		if v, ok := jsonPath(parsed, path); ok {
			res.Output[field] = v
		}
	}
	return res, nil
}

func jsonPath(v any, path string) (any, bool) {
	cur := v
	for _, p := range strings.Split(strings.TrimPrefix(path, "$."), ".") {
		if p == "" || p == "$" {
			continue
		}
		switch x := cur.(type) {
		case map[string]any:
			n, ok := x[p]
			if !ok {
				return nil, false
			}
			cur = n
		case []any:
			i, err := strconv.Atoi(p)
			if err != nil || i < 0 || i >= len(x) {
				return nil, false
			}
			cur = x[i]
		default:
			return nil, false
		}
	}
	return cur, true
}

// studioJob runs decision / connector steps designed in the studio.
// handled=false means the node is not a studio step (use the handlers).
func (s *Service) studioJob(ctx context.Context, versionID string, n *bpmn.Node, vars map[string]any) (out map[string]any, handled bool, err error) {
	if n.Ext["decision"] == "" && n.Ext["connector"] == "" {
		return nil, false, nil
	}
	v, err := s.S.GetVersion(ctx, versionID)
	if err != nil {
		return nil, true, err
	}
	if v.Model == nil {
		return nil, true, fmt.Errorf("step %s refers to a studio artefact but the version has no case model", n.ID)
	}
	if k := n.Ext["decision"]; k != "" {
		for i := range v.Model.Decisions {
			if v.Model.Decisions[i].Key == k {
				out, _, err := EvalDecision(&v.Model.Decisions[i], vars)
				return out, true, err
			}
		}
		return nil, true, fmt.Errorf("decision table %q not found", k)
	}
	k := n.Ext["connector"]
	for i := range v.Model.Connectors {
		if v.Model.Connectors[i].Key == k {
			r, err := CallConnector(ctx, &v.Model.Connectors[i], vars, IdempotencyKey(ctx))
			if err != nil {
				return nil, true, err
			}
			return r.Output, true, nil
		}
	}
	return nil, true, fmt.Errorf("connector %q not found", k)
}

// TestDecision / TestConnector let designers try artefacts in the studio.
func (s *Service) TestDecision(ctx context.Context, by Actor, vid, key string, vars map[string]any) (map[string]any, error) {
	if !by.Has(RoleDesigner) {
		return nil, ErrForbidden
	}
	v, err := s.S.GetVersion(ctx, vid)
	if err != nil || v.Model == nil {
		return nil, invalid("no case model")
	}
	for i := range v.Model.Decisions {
		if v.Model.Decisions[i].Key == key {
			out, hits, err := EvalDecision(&v.Model.Decisions[i], vars)
			if err != nil {
				return nil, invalid("%v", err)
			}
			rows := []int{}
			for _, h := range hits {
				rows = append(rows, h+1)
			}
			return map[string]any{"output": out, "rows": rows}, nil
		}
	}
	return nil, store.ErrNotFound
}

func (s *Service) TestConnector(ctx context.Context, by Actor, c *store.Connector, vars map[string]any) (*ConnectorResult, error) {
	if !by.Has(RoleDesigner) {
		return nil, ErrForbidden
	}
	r, err := CallConnector(ctx, c, vars, "studio-test")
	if err != nil {
		r.Error = err.Error()
	}
	return r, nil
}

// hostAllowed checks a comma-separated allow list (exact host or ".suffix").
func hostAllowed(host, allow string) bool {
	for _, a := range strings.Split(allow, ",") {
		a = strings.TrimSpace(strings.ToLower(a))
		h := strings.ToLower(host)
		if a != "" && (h == a || (strings.HasPrefix(a, ".") && strings.HasSuffix(h, a))) {
			return true
		}
	}
	return false
}
