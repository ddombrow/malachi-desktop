package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ddombrow/malachi-memory/memory"
	"github.com/ddombrow/malachi/coding"
)

// Memory: the agent reaches malachi-memory as an MCP server that is this
// very binary run with memoryServerArg (main dispatches it before the GUI
// starts), handed to each session through coding.Options.ExtraMCPServers.
// malachi sandboxes it like any MCP server. The app's Memory panel opens the
// same database directly; malachi-memory opens it only around each call, so
// both, and any other malachi-memory servers, can share it.

// memoryServerArg makes the binary serve malachi-memory on stdio.
const memoryServerArg = "--memory-mcp"

// memoryServerName is the MCP server's name in sessions; the agent sees its
// tools as mcp__memory__remember and so on. A server of the same name in
// the user's mcp.json wins.
const memoryServerName = "memory"

// runMemoryServer serves malachi-memory on stdin/stdout if the process was
// started for that, and exits; otherwise it returns.
func runMemoryServer() {
	if len(os.Args) != 2 || os.Args[1] != memoryServerArg {
		return
	}
	path := os.Getenv("MEMORY_DB_PATH")
	if path == "" {
		fmt.Fprintln(os.Stderr, "malachi-desktop memory: MEMORY_DB_PATH is not set")
		os.Exit(2)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "malachi-desktop memory:", err)
		os.Exit(1)
	}
	db, err := memory.NewDb(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "malachi-desktop memory:", err)
		os.Exit(1)
	}
	if err := memory.Serve(db, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "malachi-desktop memory:", err)
		os.Exit(1)
	}
	os.Exit(0)
}

// memoryDBPath keeps the database with the app's other data (appDataDir),
// so the app is self-contained: ~/.local/share/malachi-desktop/memory on
// Linux, ~/Library/Application Support/malachi-desktop/memory on macOS. A
// malachi-memory server configured for the CLI can share it by pointing
// MEMORY_DB_PATH here.
func memoryDBPath() (string, error) {
	base, err := appDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "memory", "memory.pebble"), nil
}

// memoryServerConfig is the MCP server added to sessions while memory is on.
func memoryServerConfig() (coding.MCPServerConfig, error) {
	exe, err := os.Executable()
	if err != nil {
		return coding.MCPServerConfig{}, err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	db, err := memoryDBPath()
	if err != nil {
		return coding.MCPServerConfig{}, err
	}
	// The sandboxed server may write only inside its writable root, so it
	// cannot create that root itself: the app creates it first.
	if err := os.MkdirAll(filepath.Dir(db), 0o700); err != nil {
		return coding.MCPServerConfig{}, err
	}
	direct := false
	return coding.MCPServerConfig{
		Command:     exe,
		Args:        []string{memoryServerArg},
		Description: "Notes about the user and their projects that persist between sessions",
		Env:         map[string]string{"MEMORY_DB_PATH": db},
		// The server process may read its own binary's directory (the home
		// directory is private in the sandbox) and write only the database.
		Readable: []string{filepath.Dir(exe)},
		Writable: []string{filepath.Dir(db)},
		Deferred: &direct, // offer remember/recall directly, not via scripts
	}, nil
}

// ---- the Memory panel's view of the database ----

// MemoryItem is one memory as the panel shows it.
type MemoryItem struct {
	ID      string   `json:"id"` // 26-character ULID
	Text    string   `json:"text"`
	Topics  []string `json:"topics"`
	Created int64    `json:"created"` // Unix ms
}

// TopicCount is a topic and how many memories are about it.
type TopicCount struct {
	Name  string `json:"name"`
	Count string `json:"count"`
}

// MemoryState is what the panel shows in its header.
type MemoryState struct {
	Enabled bool   `json:"enabled"`
	Path    string `json:"path"`
	Exists  bool   `json:"exists"` // the database has been created
}

var (
	memDBMu sync.Mutex
	memDB   *memory.Db
)

// memoryDB opens (once) the shared database, creating it if create is set;
// without create, a database that does not exist yet yields nil.
func memoryDB(create bool) (*memory.Db, error) {
	memDBMu.Lock()
	defer memDBMu.Unlock()
	if memDB != nil {
		return memDB, nil
	}
	path, err := memoryDBPath()
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err != nil {
		if !create {
			return nil, nil
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, err
		}
	}
	db, err := memory.NewDb(path)
	if err != nil {
		return nil, err
	}
	memDB = db
	return db, nil
}

// MemoryState reports whether memory is on and where the database is.
func (m *MalachiService) MemoryState() (MemoryState, error) {
	st, _ := m.GetSettings()
	path, err := memoryDBPath()
	if err != nil {
		return MemoryState{}, err
	}
	_, statErr := os.Stat(path)
	return MemoryState{Enabled: st.Memory == "on", Path: path, Exists: statErr == nil}, nil
}

// MemoryList returns memories matching query (BM25 search) or about topic,
// else the most recent, up to limit.
func (m *MalachiService) MemoryList(query, topic string, limit int) ([]MemoryItem, error) {
	out := []MemoryItem{}
	db, err := memoryDB(false)
	if err != nil || db == nil {
		return out, err
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	var found []memory.Memory
	_, err = db.View(func(s *memory.Store) (string, error) {
		var err error
		switch {
		case strings.TrimSpace(query) != "":
			found, err = s.Search(query, limit)
		case topic != "":
			found, err = s.About(topic, limit)
		default:
			found, err = s.Recent(limit)
		}
		return "", err
	})
	if err != nil {
		return out, err
	}
	for _, f := range found {
		topics := f.Topics
		if topics == nil {
			topics = []string{}
		}
		out = append(out, MemoryItem{ID: f.ID.String(), Text: f.Text, Topics: topics, Created: f.Created * 1000})
	}
	return out, nil
}

// MemoryTopics lists every topic with its count.
func (m *MalachiService) MemoryTopics() ([]TopicCount, error) {
	out := []TopicCount{}
	db, err := memoryDB(false)
	if err != nil || db == nil {
		return out, err
	}
	var pairs [][2]string
	_, err = db.View(func(s *memory.Store) (string, error) {
		var err error
		pairs, err = s.Topics("")
		return "", err
	})
	for _, p := range pairs {
		out = append(out, TopicCount{Name: p[0], Count: p[1]})
	}
	return out, err
}

// MemoryRemember stores a memory the user typed into the panel.
func (m *MalachiService) MemoryRemember(text string, topics []string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", errors.New("nothing to remember")
	}
	db, err := memoryDB(true)
	if err != nil {
		return "", err
	}
	var id memory.ID
	_, err = db.With(func(s *memory.Store) (string, error) {
		var err error
		id, err = s.Remember(text, topics)
		return "", err
	})
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

// MemoryForget deletes a memory.
func (m *MalachiService) MemoryForget(idText string) error {
	id, err := memory.ParseID(idText)
	if err != nil {
		return err
	}
	db, err := memoryDB(false)
	if err != nil || db == nil {
		return err
	}
	_, err = db.With(func(s *memory.Store) (string, error) {
		_, err := s.Forget(id)
		return "", err
	})
	return err
}
