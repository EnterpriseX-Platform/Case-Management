package engine

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/expr-lang/expr"
)

// unwrap strips JUEL-style ${...} / #{...} wrappers used by most BPMN tools.
func unwrap(s string) string {
	s = strings.TrimSpace(s)
	if (strings.HasPrefix(s, "${") || strings.HasPrefix(s, "#{")) && strings.HasSuffix(s, "}") {
		s = s[2 : len(s)-1]
	}
	return strings.TrimSpace(s)
}

func evalAny(src string, vars map[string]any) (any, error) {
	env := map[string]any{}
	for k, v := range vars {
		env[k] = v
	}
	prog, err := expr.Compile(unwrap(src), expr.Env(env), expr.AllowUndefinedVariables())
	if err != nil {
		return nil, fmt.Errorf("expression %q: %w", src, err)
	}
	return expr.Run(prog, env)
}

func evalBool(src string, vars map[string]any) (bool, error) {
	v, err := evalAny(src, vars)
	if err != nil {
		return false, err
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("expression %q is %T, want bool", src, v)
	}
	return b, nil
}

// runScript executes "name = expression" lines (one per line or ';').
func runScript(src string, vars map[string]any) (map[string]any, error) {
	out := map[string]any{}
	env := map[string]any{}
	for k, v := range vars {
		env[k] = v
	}
	for _, line := range strings.FieldsFunc(src, func(r rune) bool { return r == '\n' || r == ';' }) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		name, ex, ok := strings.Cut(line, "=")
		name = strings.TrimSpace(name)
		if !ok || !identRe.MatchString(name) || strings.HasPrefix(ex, "=") {
			return nil, fmt.Errorf("script: want `name = expression`, got %q", line)
		}
		v, err := evalAny(ex, env)
		if err != nil {
			return nil, err
		}
		env[name], out[name] = v, v
	}
	return out, nil
}

var identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

var isoDur = regexp.MustCompile(`^P(?:(\d+)Y)?(?:(\d+)M)?(?:(\d+)W)?(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:(\d+(?:\.\d+)?)S)?)?$`)

// ParseDuration parses ISO-8601 durations such as P10D, PT4H, P1DT12H.
// Years and months are approximated as 365 and 30 days.
func ParseDuration(s string) (time.Duration, error) {
	m := isoDur.FindStringSubmatch(strings.ToUpper(strings.TrimSpace(s)))
	if m == nil || s == "P" || s == "PT" {
		return 0, fmt.Errorf("invalid ISO-8601 duration %q", s)
	}
	n := func(i int) float64 { f, _ := strconv.ParseFloat(m[i], 64); return f }
	day := 24 * time.Hour
	d := time.Duration(n(1)*365)*day + time.Duration(n(2)*30)*day + time.Duration(n(3)*7)*day +
		time.Duration(n(4))*day + time.Duration(n(5))*time.Hour + time.Duration(n(6))*time.Minute +
		time.Duration(n(7)*float64(time.Second))
	return d, nil
}

// EvalBool evaluates a boolean expression against case variables.
func EvalBool(src string, vars map[string]any) (bool, error) { return evalBool(src, vars) }
