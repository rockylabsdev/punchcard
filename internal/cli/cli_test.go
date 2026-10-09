package cli_test

import (
	"bytes"
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/j-dsmith/punchcard/internal/cli"
)

type result struct {
	code           int
	stdout, stderr string
}

func run(t *testing.T, env map[string]string, args ...string) result {
	t.Helper()
	var out, errb bytes.Buffer
	code := cli.Run(args, cli.Env{
		Stdout:  &out,
		Stderr:  &errb,
		Getenv:  func(k string) string { return env[k] },
		Version: "1.2.3",
	})
	return result{code, out.String(), errb.String()}
}

func decode(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("stdout is not one JSON envelope: %v\n%s", err, s)
	}
	return m
}

func TestVersionJSONPrintsTheEnvelope(t *testing.T) {
	want := map[string]any{
		"v": 1.0, "ok": true, "outcome": "ok", "command": "version",
		"data": map[string]any{
			"version": "1.2.3", "contract": 1.0, "schemaVersion": 1.0,
			"go": runtime.Version(), "platform": runtime.GOOS + "/" + runtime.GOARCH,
		},
		"events": []any{},
		"error":  nil,
	}
	for name, r := range map[string]result{
		"--json flag":      run(t, nil, "version", "--json"),
		"PUNCHCARD_JSON=1": run(t, map[string]string{"PUNCHCARD_JSON": "1"}, "version"),
	} {
		t.Run(name, func(t *testing.T) {
			if r.code != 0 {
				t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
			}
			if d := cmp.Diff(want, decode(t, r.stdout)); d != "" {
				t.Errorf("envelope (-want +got):\n%s", d)
			}
		})
	}
}

func TestVersionPlain(t *testing.T) {
	r := run(t, nil, "version")
	if r.code != 0 {
		t.Fatalf("exit %d", r.code)
	}
	if !strings.HasPrefix(r.stdout, "punchcard 1.2.3") {
		t.Errorf("stdout = %q, want it to start with %q", r.stdout, "punchcard 1.2.3")
	}
}

func TestUnknownCommandIsUsageNeverTwo(t *testing.T) {
	r := run(t, nil, "frobnicate")
	if r.code != 64 {
		t.Errorf("exit %d, want 64", r.code)
	}
	if r.stdout != "" || r.stderr == "" {
		t.Errorf("human errors go to stderr: stdout %q stderr %q", r.stdout, r.stderr)
	}

	r = run(t, nil, "frobnicate", "--json")
	if r.code != 64 {
		t.Errorf("exit %d, want 64", r.code)
	}
	env := decode(t, r.stdout)
	if env["ok"] != false || env["outcome"] != "usage" {
		t.Errorf("envelope = %v", env)
	}
	if e, _ := env["error"].(map[string]any); e["code"] != "usage" || e["message"] == "" {
		t.Errorf("error = %v", env["error"])
	}
}
