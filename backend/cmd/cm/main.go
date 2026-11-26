// Command cm is the developer CLI: keep case-management apps as files in git
// and move them between environments.
//
//	cm pull <case-type> [dir]   download the latest version as files (flow, model, forms, screens)
//	cm push <dir|file.cmpack>   upload as a new draft version (preview first; --yes to commit)
//	cm validate <file.bpmn>     check a BPMN file against the engine
//	cm list                     list case types and their versions
//
// Environment: CM_URL (default http://localhost:8080), CM_TOKEN (OIDC bearer)
// or, in development, CM_USER and CM_ROLES.
package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "pull":
		err = pull(arg(2, ""), arg(3, ""))
	case "push":
		err = push(arg(2, "."), hasFlag("--yes"))
	case "validate":
		err = validate(arg(2, ""))
	case "list":
		err = list()
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "cm:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: cm pull <case-type> [dir] | cm push <dir|file.cmpack> [--yes] | cm validate <file.bpmn> | cm list")
	os.Exit(2)
}

func arg(i int, d string) string {
	n := 0
	for _, a := range os.Args {
		if strings.HasPrefix(a, "--") {
			continue
		}
		if n == i {
			return a
		}
		n++
	}
	if d == "" && i == 2 {
		usage()
	}
	return d
}

func hasFlag(f string) bool {
	for _, a := range os.Args {
		if a == f {
			return true
		}
	}
	return false
}

func base() string {
	if u := os.Getenv("CM_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	return "http://localhost:8080"
}

func do(method, path string, body io.Reader, ctype string) ([]byte, error) {
	req, err := http.NewRequest(method, base()+path, body)
	if err != nil {
		return nil, err
	}
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	if t := os.Getenv("CM_TOKEN"); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	} else {
		req.Header.Set("X-User", envOr("CM_USER", "cli"))
		req.Header.Set("X-Roles", envOr("CM_ROLES", "cm:designer,cm:admin"))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		var e struct{ Error string }
		if json.Unmarshal(b, &e) == nil && e.Error != "" {
			return nil, fmt.Errorf("%s", e.Error)
		}
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return b, nil
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func pull(key, dir string) error {
	if dir == "" {
		dir = key
	}
	b, err := do("GET", "/api/processes/"+key+"/export", nil, "")
	if err != nil {
		return err
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return err
	}
	for _, f := range zr.File {
		p := filepath.Join(dir, filepath.Clean("/"+f.Name)) // no path escape
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return err
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			return err
		}
		fmt.Println("  ", p)
	}
	return nil
}

func zipDir(dir string) ([]byte, error) {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || strings.HasPrefix(info.Name(), ".") {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		w, err := z.Create(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	})
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), z.Close()
}

func push(src string, commit bool) error {
	var data []byte
	var err error
	name := filepath.Base(src)
	if st, e := os.Stat(src); e == nil && st.IsDir() {
		data, err = zipDir(src)
		name += ".cmpack"
	} else {
		data, err = os.ReadFile(src)
	}
	if err != nil {
		return err
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", name)
	fw.Write(data)
	mw.WriteField("kind", "pack")
	if commit {
		mw.WriteField("commit", "1")
	}
	mw.Close()
	b, err := do("POST", "/api/imports", &body, mw.FormDataContentType())
	if err != nil {
		return err
	}
	var out map[string]any
	json.Unmarshal(b, &out)
	if !commit {
		fmt.Printf("preview: %v\n", out["summary"])
		if w, ok := out["warnings"].([]any); ok {
			for _, x := range w {
				fmt.Println("  warning:", x)
			}
		}
		fmt.Println("run again with --yes to import as a new draft version")
		return nil
	}
	fmt.Printf("imported: %v\n", out["summary"])
	return nil
}

func validate(file string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	b, err := do("POST", "/api/validate/bpmn", bytes.NewReader(data), "application/xml")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

func list() error {
	b, err := do("GET", "/api/processes", nil, "")
	if err != nil {
		return err
	}
	var ps []struct{ Key, Name string }
	json.Unmarshal(b, &ps)
	for _, p := range ps {
		vb, _ := do("GET", "/api/processes/"+p.Key+"/versions", nil, "")
		var vs []struct {
			No         int
			Status     string
			FiscalYear int `json:"fiscalYear"`
		}
		json.Unmarshal(vb, &vs)
		fmt.Printf("%-32s %s\n", p.Key, p.Name)
		for _, v := range vs {
			fmt.Printf("    v%-3d %-11s %d\n", v.No, v.Status, v.FiscalYear)
		}
	}
	return nil
}
