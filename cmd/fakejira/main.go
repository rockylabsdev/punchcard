// Command fakejira is a stand-in for Jira Cloud on localhost, so every
// surface of Punchcard (setup, the CLI, the TUI, the Claude Code plugin)
// can be tried without a real account. It speaks the four calls
// Punchcard makes, knows a handful of tickets, logs every request, and
// can be switched into failure modes while running.
//
//	go run ./cmd/fakejira
//
// then point setup at http://127.0.0.1:8181 with any email and any token.
// See docs/guide/sandbox.md.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// The tickets this Jira knows. Anything else is a 404, as a typo would be.
var tickets = map[string]string{
	"PUN-1":   "Set up the sandbox Jira",
	"PUN-2":   "Wire the worklog POST",
	"PUN-3":   "Stop dialog: duration parsing and the live preview",
	"OPS-77":  "Rotate staging certs",
	"DATA-9":  "Backfill the events table",
	"WEB-412": "Investigate intermittent CI timeouts",
}

// modes are the ways this Jira can misbehave, switched at runtime with
// GET /_fake/mode?set=<mode>.
var modes = map[string]string{
	"ok":   "every call succeeds",
	"401":  "every call is rejected: credentials invalid",
	"503":  "worklog POSTs fail with 503; reads still work",
	"slow": "every call takes 20 s, longer than any Punchcard budget: unreachable",
}

type server struct {
	mu       sync.Mutex
	mode     string
	worklogs []worklog
	nextID   int
}

type worklog struct {
	ID               string `json:"id"`
	Key              string `json:"key"`
	Started          string `json:"started"`
	TimeSpentSeconds int    `json:"timeSpentSeconds"`
	Comment          string `json:"comment,omitempty"`
	ReceivedAt       string `json:"receivedAt"`
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8181", "address to listen on")
	mode := flag.String("mode", "ok", "starting mode: ok, 401, 503 or slow")
	flag.Parse()
	if _, ok := modes[*mode]; !ok {
		fmt.Fprintf(os.Stderr, "fakejira: unknown mode %q\n", *mode)
		os.Exit(64)
	}
	s := &server{mode: *mode, nextID: 110000}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/serverInfo", s.serverInfo)
	mux.HandleFunc("GET /rest/api/3/myself", s.myself)
	mux.HandleFunc("GET /rest/api/3/issue/{key}", s.issue)
	mux.HandleFunc("POST /rest/api/3/issue/{key}/worklog", s.postWorklog)
	mux.HandleFunc("GET /_fake/mode", s.setMode)
	mux.HandleFunc("GET /_fake/worklogs", s.listWorklogs)
	mux.HandleFunc("DELETE /_fake/worklogs", s.clearWorklogs)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.reply(w, r, 404, errorBody("no such endpoint: "+r.Method+" "+r.URL.Path))
	})

	keys := make([]string, 0, len(tickets))
	for k := range tickets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Printf("fakejira on http://%s (mode %s)\n\n", *addr, s.mode)
	fmt.Printf("  punchcard setup   site: http://%s   email: anyone@example.com   token: anything\n", *addr)
	fmt.Printf("  tickets: %s (any other key is a 404)\n", strings.Join(keys, ", "))
	fmt.Printf("  modes:   curl 'http://%s/_fake/mode?set=401'   (ok, 401, 503, slow)\n", *addr)
	fmt.Printf("  posted:  curl http://%s/_fake/worklogs\n\n", *addr)
	log.SetFlags(log.Ltime)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

// gate applies the current mode before a handler answers: a 401, or the
// slow wait. It reports whether the handler should go on.
func (s *server) gate(w http.ResponseWriter, r *http.Request) bool {
	s.mu.Lock()
	mode := s.mode
	s.mu.Unlock()
	switch mode {
	case "401":
		s.reply(w, r, 401, errorBody("Client must be authenticated to access this resource."))
		return false
	case "slow":
		time.Sleep(20 * time.Second)
	}
	return true
}

func (s *server) serverInfo(w http.ResponseWriter, r *http.Request) {
	if !s.gate(w, r) {
		return
	}
	s.reply(w, r, 200, map[string]any{"baseUrl": "http://" + r.Host, "deploymentType": "Cloud", "version": "1001.0.0-SNAPSHOT"})
}

func (s *server) myself(w http.ResponseWriter, r *http.Request) {
	if !s.gate(w, r) {
		return
	}
	email, _, _ := r.BasicAuth()
	s.reply(w, r, 200, map[string]any{
		"accountId": "712020:11111111-2222-3333-4444-555555555555", "emailAddress": email, "displayName": "Sandbox Tester",
	})
}

func (s *server) issue(w http.ResponseWriter, r *http.Request) {
	if !s.gate(w, r) {
		return
	}
	key := r.PathValue("key")
	summary, ok := tickets[key]
	if !ok {
		s.reply(w, r, 404, errorBody("Issue does not exist or you do not have permission to see it."))
		return
	}
	s.reply(w, r, 200, map[string]any{"id": "100001", "key": key, "fields": map[string]any{"summary": summary}})
}

func (s *server) postWorklog(w http.ResponseWriter, r *http.Request) {
	if !s.gate(w, r) {
		return
	}
	key := r.PathValue("key")
	if _, ok := tickets[key]; !ok {
		s.reply(w, r, 404, errorBody("Issue does not exist or you do not have permission to see it."))
		return
	}
	var body struct {
		TimeSpentSeconds int    `json:"timeSpentSeconds"`
		Started          string `json:"started"`
		Comment          *struct {
			Content []struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"content"`
		} `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.reply(w, r, 400, errorBody("bad worklog body: "+err.Error()))
		return
	}
	s.mu.Lock()
	if s.mode == "503" {
		s.mu.Unlock()
		s.reply(w, r, 503, errorBody("Service unavailable"))
		return
	}
	s.nextID++
	wl := worklog{ID: fmt.Sprint(s.nextID), Key: key, Started: body.Started, TimeSpentSeconds: body.TimeSpentSeconds,
		ReceivedAt: time.Now().Format(time.RFC3339)}
	if body.Comment != nil {
		for _, p := range body.Comment.Content {
			for _, t := range p.Content {
				wl.Comment += t.Text
			}
		}
	}
	s.worklogs = append(s.worklogs, wl)
	s.mu.Unlock()
	log.Printf("  worklog %s on %s: %ds %q", wl.ID, key, wl.TimeSpentSeconds, wl.Comment)
	s.reply(w, r, 201, map[string]any{"id": wl.ID, "issueId": "100001", "started": wl.Started, "timeSpentSeconds": wl.TimeSpentSeconds})
}

func (s *server) setMode(w http.ResponseWriter, r *http.Request) {
	m := r.URL.Query().Get("set")
	if m == "" {
		s.mu.Lock()
		m = s.mode
		s.mu.Unlock()
		s.reply(w, r, 200, map[string]any{"mode": m, "modes": modes})
		return
	}
	if _, ok := modes[m]; !ok {
		s.reply(w, r, 400, errorBody("unknown mode "+m))
		return
	}
	s.mu.Lock()
	s.mode = m
	s.mu.Unlock()
	log.Printf("  mode is now %s: %s", m, modes[m])
	s.reply(w, r, 200, map[string]any{"mode": m, "means": modes[m]})
}

func (s *server) listWorklogs(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	out := append([]worklog{}, s.worklogs...)
	s.mu.Unlock()
	s.reply(w, r, 200, out)
}

func (s *server) clearWorklogs(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	n := len(s.worklogs)
	s.worklogs = nil
	s.mu.Unlock()
	s.reply(w, r, 200, map[string]any{"deleted": n})
}

func errorBody(msg string) map[string]any {
	return map[string]any{"errorMessages": []string{msg}, "errors": map[string]string{}}
}

// reply writes a JSON body and logs the request the way a tester wants
// to see it: method, path, status.
func (s *server) reply(w http.ResponseWriter, r *http.Request, status int, body any) {
	b, _ := json.Marshal(body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(b)
	if !strings.HasPrefix(r.URL.Path, "/_fake/") {
		log.Printf("%d %s %s", status, r.Method, r.URL.Path)
	}
}
