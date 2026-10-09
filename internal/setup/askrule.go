package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/j-dsmith/punchcard/internal/config"
)

// AskRule is the permissions.ask entry that gates an agent's
// `punchcard stop --yes` even with the plugin disabled (ADR 0005).
const AskRule = "Bash(punchcard stop *--yes*)"

// SettingsPath is Claude Code's user settings file:
// $CLAUDE_CONFIG_DIR/settings.json, else ~/.claude/settings.json; ""
// with neither set.
func SettingsPath(getenv func(string) string) string {
	if d := getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return filepath.Join(d, "settings.json")
	}
	if h := getenv("HOME"); h != "" {
		return filepath.Join(h, ".claude", "settings.json")
	}
	return ""
}

type askState int

const (
	askMissing     askState = iota // the rule can be added (out holds the new file)
	askPresent                     // already in permissions.ask
	askUnparseable                 // not JSON we can edit safely: left alone
)

func (s askState) String() string {
	return [...]string{"missing", "present", "unparseable"}[s]
}

// askPlan is what the ask-rule step found in the settings file and,
// when the rule is missing, the file to write.
type askPlan struct {
	state  askState
	out    []byte // the new file, when missing
	old    []byte // the file as read; nil when there was none
	exists bool
	mode   fs.FileMode
}

// readAskRule reads the settings file (none is fine) and plans the
// edit. Only a read failure other than "no file" is an error.
func readAskRule(path string) (askPlan, error) {
	b, err := os.ReadFile(path)
	p := askPlan{mode: 0o644}
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return p, err
	default:
		p.exists, p.old = true, b
		if fi, err := os.Stat(path); err == nil {
			p.mode = fi.Mode().Perm()
		}
	}
	p.state, p.out = planAskRule(b)
	return p, nil
}

// planAskRule is the pure edit: the rule appended to permissions.ask,
// every other key kept, in its order. Empty input is an empty object.
func planAskRule(b []byte) (askState, []byte) {
	if len(bytes.TrimSpace(b)) == 0 {
		b = []byte("{}")
	}
	top, err := decodeObject(b)
	if err != nil {
		return askUnparseable, nil
	}
	perms := object{}
	i := top.index("permissions")
	if i >= 0 {
		if perms, err = decodeObject(top[i].val); err != nil {
			return askUnparseable, nil
		}
	}
	var ask [][]byte
	j := perms.index("ask")
	if j >= 0 {
		var raw []json.RawMessage
		if err := json.Unmarshal(perms[j].val, &raw); err != nil || raw == nil {
			return askUnparseable, nil
		}
		for _, r := range raw {
			ask = append(ask, r)
		}
		for _, r := range ask {
			var s string
			if json.Unmarshal(r, &s) == nil && s == AskRule {
				return askPresent, nil
			}
		}
	}
	ask = append(ask, quote(AskRule))
	perms = perms.set(j, "ask", append(append([]byte("["), bytes.Join(ask, []byte(","))...), ']'))
	top = top.set(i, "permissions", perms.encode())
	var out bytes.Buffer
	if err := json.Indent(&out, top.encode(), "", "  "); err != nil {
		return askUnparseable, nil
	}
	out.WriteByte('\n')
	return askMissing, out.Bytes()
}

// writeAskRule writes a planned edit: the old file copied to .bak, then
// the new one put in place atomically. Through a symlink, the file it
// points at is replaced and the link kept.
func writeAskRule(path string, p askPlan) error {
	if p.state != askMissing {
		return fmt.Errorf("nothing to write (%v)", p.state)
	}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	if p.exists {
		if err := config.WriteFileAtomic(path+".bak", p.old, p.mode); err != nil {
			return err
		}
	}
	return config.WriteFileAtomic(path, p.out, p.mode)
}

// object is a JSON object with its keys in file order.
type object []member

type member struct {
	key string
	val json.RawMessage
}

func (o object) index(key string) int {
	for i, m := range o {
		if m.key == key {
			return i
		}
	}
	return -1
}

// set replaces member i, or appends key when i < 0.
func (o object) set(i int, key string, val json.RawMessage) object {
	if i >= 0 {
		o[i].val = val
		return o
	}
	return append(o, member{key, val})
}

func (o object) encode() []byte {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, m := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(quote(m.key))
		b.WriteByte(':')
		b.Write(m.val)
	}
	b.WriteByte('}')
	return b.Bytes()
}

// decodeObject reads one JSON object, keeping key order and each
// value's raw bytes. Anything after it is an error.
func decodeObject(b []byte) (object, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	t, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := t.(json.Delim); !ok || d != '{' {
		return nil, errors.New("not an object")
	}
	o := object{}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, err
		}
		k, _ := t.(string)
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, err
		}
		o = append(o, member{k, v})
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("more after the object")
	}
	return o, nil
}

// quote is s as a JSON string, without HTML escaping.
func quote(s string) []byte {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return bytes.TrimSuffix(b.Bytes(), []byte("\n"))
}
