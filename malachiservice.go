package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ddombrow/malachi/coding"
	"github.com/ddombrow/malachi/rpc"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Events emitted to the frontend.
const (
	evEvent  = "malachi:event"  // one agent/session event, raw JSON string
	evStderr = "malachi:stderr" // a note for the log panel
	evExit   = "malachi:exit"   // the session server stopped; data is the reason
)

func init() {
	application.RegisterEvent[string](evEvent)
	application.RegisterEvent[string](evStderr)
	application.RegisterEvent[string](evExit)
}

// MalachiService runs malachi in-process: a coding.Session served by
// malachi's own rpc.Server over in-memory pipes. Commands go in through Send
// and their responses come back to the caller; everything else is emitted to
// the frontend as an event. Speaking the RPC protocol rather than calling
// coding.Session directly keeps the frontend identical to a client of
// `malachi -mode rpc`, and keeps every rule about what may run in malachi.
type MalachiService struct {
	mu      sync.Mutex
	proc    *rpcProc
	nextID  atomic.Int64
	initErr string // loading ~/.malachi/.env failed
}

// rpcProc is one running session server.
type rpcProc struct {
	in      *io.PipeWriter // commands, one JSON object per line
	sv      *rpc.Server    // for what the protocol does not carry (diff.go)
	cancel  context.CancelFunc
	cwd     string
	wmu     sync.Mutex
	pmu     sync.Mutex
	pending map[string]chan rpcResponse
	done    chan struct{} // closed once the server and its output have ended
}

type rpcResponse struct {
	Command string          `json:"command"`
	Success bool            `json:"success"`
	ID      json.RawMessage `json:"id"`
	Data    json.RawMessage `json:"data"`
	Error   string          `json:"error"`
}

// OpenOptions choose how a session starts.
type OpenOptions struct {
	Cwd      string `json:"cwd"`
	Continue bool   `json:"continue"` // continue the latest session there
	Resume   string `json:"resume"`   // a session file path
	Model    string `json:"model"`
	Thinking string `json:"thinking"`
}

// Info is what the frontend needs before a session is open.
type Info struct {
	Version    string `json:"version"`    // malachi's
	AppVersion string `json:"appVersion"` // this app's, from build/config.yml
	InitErr    string `json:"initErr,omitempty"`
	DefaultCwd string `json:"defaultCwd"`
	// ChatDir is where project-less chats run: an empty directory of the
	// app's own, so the sandbox's writable root holds nothing of the user's.
	ChatDir    string `json:"chatDir"`
	ChatDirErr string `json:"chatDirErr,omitempty"`
	Cwd        string `json:"cwd"`
	Running    bool   `json:"running"`
}

func (m *MalachiService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	// Provider keys may live in ~/.malachi/.env, as for the CLI. malachi
	// withholds them from the commands it runs.
	if err := coding.LoadDotEnv(filepath.Join(coding.Home(), ".env")); err != nil {
		m.initErr = err.Error()
	}
	return nil
}

func (m *MalachiService) ServiceShutdown() error {
	m.Close()
	return nil
}

// Info reports malachi's version and the directories to offer.
func (m *MalachiService) Info() Info {
	in := Info{Version: coding.Version, AppVersion: appVersion, InitErr: m.initErr, DefaultCwd: defaultCwd()}
	if d, err := chatDir(); err != nil {
		in.ChatDirErr = err.Error()
	} else {
		in.ChatDir = d
	}
	m.mu.Lock()
	if m.proc != nil {
		in.Cwd, in.Running = m.proc.cwd, true
	}
	m.mu.Unlock()
	return in
}

// PickDirectory asks the user for a project directory; "" if cancelled.
func (m *MalachiService) PickDirectory(start string) (string, error) {
	d := application.Get().Dialog.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false).
		CanCreateDirectories(true).
		SetTitle("Open project")
	if start != "" {
		d = d.SetDirectory(start)
	}
	return d.PromptForSingleSelection()
}

// Open stops any running session and starts one in opts.Cwd.
func (m *MalachiService) Open(opts OpenOptions) error {
	cwd, err := filepath.Abs(expandHome(opts.Cwd))
	if err != nil {
		return err
	}
	if st, err := os.Stat(cwd); err != nil || !st.IsDir() {
		return fmt.Errorf("not a directory: %s", cwd)
	}
	m.Close()

	// The model and thinking level the user last picked apply to every
	// session the app starts, unless the caller names others. Like the
	// CLI's flags, they win over the model a resumed session used.
	saved, _ := m.GetSettings()
	if opts.Model == "" {
		opts.Model = saved.Model
	}
	if opts.Thinking == "" {
		opts.Thinking = saved.Thinking
	}

	opts.Resume = resumable(opts.Resume)

	// A saved code mode this malachi does not know would make Open fail;
	// leave it to malachi's default instead.
	codeMode := saved.CodeMode
	if !slices.Contains(coding.CodeModes(), codeMode) {
		codeMode = ""
	}

	// With memory on, the session gets malachi-memory, served by this
	// binary (memory.go). A "memory" server in the user's mcp.json wins.
	var extra map[string]coding.MCPServerConfig
	if saved.Memory == "on" {
		if cfg, err := memoryServerConfig(); err == nil {
			extra = map[string]coding.MCPServerConfig{memoryServerName: cfg}
		} else {
			application.Get().Event.Emit(evStderr, "memory is on but its server could not be set up: "+err.Error())
		}
	}

	s, err := coding.Open(coding.Options{
		Cwd:             cwd,
		Model:           opts.Model,
		ThinkingLevel:   opts.Thinking,
		CodeMode:        codeMode,
		Continue:        opts.Continue && opts.Resume == "",
		Resume:          opts.Resume,
		ExtraMCPServers: extra,
	})
	if err != nil {
		return err
	}

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	sv := rpc.New(s, inR, outW)
	p := &rpcProc{in: inW, sv: sv, cancel: cancel, cwd: cwd, pending: map[string]chan rpcResponse{}, done: make(chan struct{})}
	m.mu.Lock()
	m.proc = p
	m.mu.Unlock()

	app := application.Get()
	served := make(chan string, 1)
	go func() {
		reason := "stopped"
		defer func() {
			if r := recover(); r != nil {
				path := coding.NewDiagnostics(coding.Home()).LogPanic("desktop", r, debug.Stack())
				reason = fmt.Sprintf("internal error (logged to %s): %v", path, r)
			}
			_ = outW.Close()
			served <- reason
		}()
		err := sv.Run(ctx)
		// new_session and switch_session replace the session; close the
		// last one as well as the first.
		if last := sv.Session(); last != s {
			last.Close()
		}
		s.Close()
		if err != nil {
			reason = err.Error()
		}
	}()
	go func() {
		readRecords(outR, func(line []byte) {
			var head struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(line, &head) != nil {
				return
			}
			if head.Type == "response" {
				var r rpcResponse
				if json.Unmarshal(line, &r) == nil {
					p.deliver(r)
				}
				return
			}
			app.Event.Emit(evEvent, string(line))
		})
		reason := <-served
		close(p.done)
		p.failAll("the session stopped")
		m.mu.Lock()
		current := m.proc == p
		if current {
			m.proc = nil
		}
		m.mu.Unlock()
		if current {
			app.Event.Emit(evExit, reason)
		}
	}()
	return nil
}

// resumable returns the session file to resume, or "" when there is
// nothing to resume yet: malachi reports a new session's file path before
// its first message but creates the file only then, and resuming a path
// that does not exist fails. Reopening such a session (to apply a setting)
// then starts a fresh one, which is all there was.
func resumable(path string) string {
	if path == "" {
		return ""
	}
	if st, err := os.Stat(path); err != nil || st.IsDir() {
		return ""
	}
	return path
}

// Close ends the session: closing its input makes the server abort the
// run, deliver the last events and return, as malachi does at EOF.
func (m *MalachiService) Close() {
	m.mu.Lock()
	p := m.proc
	m.proc = nil
	m.mu.Unlock()
	if p == nil {
		return
	}
	_ = p.in.Close()
	select {
	case <-p.done:
	case <-time.After(35 * time.Second):
		// The server waits up to 30s for an aborted run to stop; past
		// that, cancel and move on rather than hang the window.
		p.cancel()
		application.Get().Event.Emit(evStderr, "the previous session did not stop in time and was abandoned")
	}
}

// Send writes one command (an object with "type" and its fields) and waits
// for its response, returning the response's data. Long commands (compact,
// mcp_restart) may take minutes, so there is no timeout beyond the session
// stopping.
func (m *MalachiService) Send(cmd map[string]any) (json.RawMessage, error) {
	m.mu.Lock()
	p := m.proc
	m.mu.Unlock()
	if p == nil {
		return nil, errors.New("no session is open")
	}
	id := fmt.Sprintf("d%d", m.nextID.Add(1))
	cmd["id"] = id
	line, err := json.Marshal(cmd)
	if err != nil {
		return nil, err
	}
	ch := make(chan rpcResponse, 1)
	p.pmu.Lock()
	p.pending[id] = ch
	p.pmu.Unlock()

	p.wmu.Lock()
	_, err = p.in.Write(append(line, '\n'))
	p.wmu.Unlock()
	if err != nil {
		p.pmu.Lock()
		delete(p.pending, id)
		p.pmu.Unlock()
		return nil, err
	}
	r := <-ch
	if !r.Success {
		return nil, errors.New(r.Error)
	}
	if len(r.Data) == 0 {
		return json.RawMessage("null"), nil
	}
	return r.Data, nil
}

func (p *rpcProc) deliver(r rpcResponse) {
	var id string
	if json.Unmarshal(r.ID, &id) != nil {
		return // not one of ours (a parse error has no id)
	}
	p.pmu.Lock()
	ch := p.pending[id]
	delete(p.pending, id)
	p.pmu.Unlock()
	if ch != nil {
		ch <- r
	}
}

func (p *rpcProc) failAll(msg string) {
	p.pmu.Lock()
	defer p.pmu.Unlock()
	for id, ch := range p.pending {
		ch <- rpcResponse{Success: false, Error: msg}
		delete(p.pending, id)
	}
}

// SessionInfo describes one saved session file.
type SessionInfo struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	Modified int64  `json:"modified"` // Unix ms
	Preview  string `json:"preview"`
}

// ListSessions lists cwd's saved sessions, newest first, each with its
// first prompt. It works before the current session has written its file.
func (m *MalachiService) ListSessions(cwd string) ([]SessionInfo, error) {
	out := []SessionInfo{}
	dir := sessionsDir(cwd)
	if dir == "" {
		return out, nil
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return out, nil
	}
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		path := filepath.Join(dir, e.Name())
		out = append(out, SessionInfo{
			Path:     path,
			Name:     strings.TrimSuffix(e.Name(), ".jsonl"),
			Modified: fi.ModTime().UnixMilli(),
			Preview:  firstPrompt(path),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Modified > out[j].Modified })
	return out, nil
}

// sessionsDir is the directory malachi keeps cwd's sessions in.
func sessionsDir(cwd string) string {
	return coding.SessionsDir(coding.Home(), cwd)
}

// RecentProject is a directory malachi has had sessions in.
type RecentProject struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	LastUsed int64  `json:"lastUsed"` // Unix ms, newest session's mtime
	Sessions int    `json:"sessions"`
}

const maxRecent = 10

// RecentProjects lists the directories malachi has worked in, from any
// frontend, newest first. Each session file's first record names its cwd.
// Left out: this app's chat directory, temporary directories, directories
// under a hidden one (other apps' internals, such as ~/.local/share/x), and
// directories that no longer exist.
func (m *MalachiService) RecentProjects() []RecentProject {
	out := []RecentProject{}
	home := coding.Home()
	dirs, err := os.ReadDir(filepath.Join(home, "sessions"))
	if err != nil {
		return out
	}
	chat, _ := chatDir()
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		dir := filepath.Join(home, "sessions", d.Name())
		newest, count, last := "", 0, int64(0)
		ents, _ := os.ReadDir(dir)
		for _, e := range ents {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
				continue
			}
			fi, err := e.Info()
			if err != nil {
				continue
			}
			count++
			if t := fi.ModTime().UnixMilli(); t > last {
				newest, last = filepath.Join(dir, e.Name()), t
			}
		}
		if newest == "" {
			continue
		}
		cwd := sessionCwd(newest)
		if cwd == "" || cwd == chat || !projectLike(cwd) {
			continue
		}
		if st, err := os.Stat(cwd); err != nil || !st.IsDir() {
			continue
		}
		out = append(out, RecentProject{Path: cwd, Name: filepath.Base(cwd), LastUsed: last, Sessions: count})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastUsed > out[j].LastUsed })
	if len(out) > maxRecent {
		out = out[:maxRecent]
	}
	return out
}

// sessionCwd reads the cwd from a session file's session_info header.
func sessionCwd(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	line, err := bufio.NewReaderSize(f, 4096).ReadSlice('\n')
	if err != nil && len(line) == 0 {
		return ""
	}
	var h struct {
		Type string `json:"type"`
		Cwd  string `json:"cwd"`
	}
	if json.Unmarshal(line, &h) != nil || h.Type != "session_info" {
		return ""
	}
	return h.Cwd
}

// projectLike reports whether dir looks like something the user would open:
// not temporary, and not inside a hidden directory.
func projectLike(dir string) bool {
	for _, tmp := range []string{os.TempDir(), "/tmp", "/var/tmp"} {
		if dir == tmp || strings.HasPrefix(dir, strings.TrimSuffix(tmp, "/")+"/") {
			return false
		}
	}
	for _, part := range strings.Split(filepath.ToSlash(dir), "/") {
		if strings.HasPrefix(part, ".") {
			return false
		}
	}
	return true
}

// firstPrompt returns the start of the first user message in a session file.
func firstPrompt(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 16<<20)
	for n := 0; sc.Scan() && n < 200; n++ {
		var e struct {
			Type    string `json:"type"`
			Message struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(sc.Bytes(), &e) != nil || e.Message.Role != "user" {
			continue
		}
		var s string
		if json.Unmarshal(e.Message.Content, &s) != nil {
			var blocks []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			_ = json.Unmarshal(e.Message.Content, &blocks)
			for _, b := range blocks {
				if b.Type == "text" {
					s = b.Text
					break
				}
			}
		}
		s = strings.Join(strings.Fields(s), " ")
		if r := []rune(s); len(r) > 90 {
			s = string(r[:90]) + "…"
		}
		return s
	}
	return ""
}

// readRecords calls fn for each LF-terminated record. Only LF ends a record,
// so a U+2028 inside a JSON string survives; records can be large.
func readRecords(r io.Reader, fn func([]byte)) {
	br := bufio.NewReaderSize(r, 256*1024)
	var buf []byte
	for {
		chunk, err := br.ReadSlice('\n')
		buf = append(buf, chunk...)
		if err == bufio.ErrBufferFull {
			continue
		}
		line := bytes.TrimRight(buf, "\r\n")
		if len(bytes.TrimSpace(line)) > 0 {
			fn(append([]byte(nil), line...))
		}
		buf = buf[:0]
		if err != nil {
			return
		}
	}
}

// chatDir returns (creating it) the chat directory under appDataDir. It
// must stay outside ~/.malachi, which malachi's sandbox hides.
func chatDir() (string, error) {
	base, err := appDataDir()
	if err != nil {
		return "", err
	}
	d := filepath.Join(base, "chat")
	if err := os.MkdirAll(d, 0o755); err != nil {
		return "", err
	}
	return d, nil
}

func defaultCwd() string {
	if wd, err := os.Getwd(); err == nil && wd != "/" {
		return wd
	}
	home, _ := os.UserHomeDir()
	return home
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	return p
}
