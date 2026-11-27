package main

import "testing"

func TestParseMode(t *testing.T) {
	for in, want := range map[string]string{
		"":                 "api,engine,integration,scheduler,notify",
		"all":              "api,engine,integration,scheduler,notify",
		"worker":           "engine,integration,scheduler,notify",
		"api":              "api",
		" engine, notify ": "engine,notify",
		"scheduler,api":    "api,scheduler",
	} {
		got, err := parseMode(in)
		if err != nil || keys(got) != want {
			t.Errorf("parseMode(%q) = %q, %v; want %q", in, keys(got), err, want)
		}
	}
	if _, err := parseMode("engine,foo"); err == nil {
		t.Error("unknown service accepted")
	}
}
