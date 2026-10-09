package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
	"golang.org/x/sys/unix"
)

// TestMain lets scripts `exec punchcard`: testscript re-runs this test
// binary as a separate process that calls run().
func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){
		"punchcard": func() { os.Exit(run()) },
	})
}

func TestScripts(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir:                 "testdata/script",
		RequireExplicitExec: true,
		Setup: func(e *testscript.Env) error {
			e.Setenv("XDG_DATA_HOME", filepath.Join(e.WorkDir, "data"))
			e.Setenv("XDG_CONFIG_HOME", filepath.Join(e.WorkDir, "config"))
			e.Setenv("HOME", filepath.Join(e.WorkDir, "home"))
			return nil
		},
		Cmds: map[string]func(ts *testscript.TestScript, neg bool, args []string){
			// exitcode N prog args...: run prog and require exit code N.
			"exitcode": func(ts *testscript.TestScript, neg bool, args []string) {
				if neg || len(args) < 2 {
					ts.Fatalf("usage: exitcode N prog [args...]")
				}
				want, err := strconv.Atoi(args[0])
				ts.Check(err)
				err = ts.Exec(args[1], args[2:]...)
				got := 0
				var ee *exec.ExitError
				if errors.As(err, &ee) {
					got = ee.ExitCode()
				} else if err != nil {
					ts.Fatalf("exec: %v", err)
				}
				if got != want {
					ts.Fatalf("exit code %d, want %d", got, want)
				}
			},
			// holdlock FILE: flock FILE from the test process until the
			// script ends, as another punchcard would.
			"holdlock": func(ts *testscript.TestScript, neg bool, args []string) {
				if neg || len(args) != 1 {
					ts.Fatalf("usage: holdlock FILE")
				}
				path := ts.MkAbs(args[0])
				ts.Check(os.MkdirAll(filepath.Dir(path), 0o700))
				f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
				ts.Check(err)
				ts.Check(unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB))
				ts.Defer(func() { f.Close() })
			},
		},
	})
}
