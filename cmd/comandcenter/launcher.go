package main

import (
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// CCConfig is loaded from ~/.claudio/cc-config.json.
type CCConfig struct {
	Sessions []SessionConfig `json:"sessions"`
}

// SessionConfig declares one managed claudio --attach session.
type SessionConfig struct {
	// Name is the --name passed to claudio (also the display name in the web UI).
	Name string `json:"name"`
	// Dir is the working directory for the claudio process.
	Dir string `json:"dir"`
	// Master enables the SendToSession / SpawnSession tools (--master flag).
	Master bool `json:"master,omitempty"`
	// Agent sets the --agent persona for this session.
	Agent string `json:"agent,omitempty"`
	// Team sets the --team template for this session.
	Team string `json:"team,omitempty"`
}

// LoadCCConfig reads the config file.  Missing file is not an error — returns empty config.
func LoadCCConfig(path string) (CCConfig, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return CCConfig{}, nil
	}
	if err != nil {
		return CCConfig{}, err
	}
	var cfg CCConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return CCConfig{}, err
	}
	return cfg, nil
}

// SessionLauncher spawns and supervises claudio --attach child processes.
type SessionLauncher struct {
	cfg       CCConfig
	serverURL string
	password  string

	mu    sync.Mutex
	procs map[string]*managedProc // keyed by session name
	stop  chan struct{}
	wg    sync.WaitGroup
}

type managedProc struct {
	cfg SessionConfig
	cmd *exec.Cmd
}

func NewSessionLauncher(cfg CCConfig, serverURL, password string) *SessionLauncher {
	return &SessionLauncher{
		cfg:       cfg,
		serverURL: serverURL,
		password:  password,
		procs:     make(map[string]*managedProc),
		stop:      make(chan struct{}),
	}
}

// Start spawns all configured sessions.  Each session is supervised and restarted on exit.
func (l *SessionLauncher) Start() {
	for _, s := range l.cfg.Sessions {
		s := s // capture
		l.wg.Add(1)
		go l.supervise(s)
	}
}

// Stop signals all supervisors to exit and kills their child processes.
func (l *SessionLauncher) Stop() {
	close(l.stop)
	l.mu.Lock()
	for _, mp := range l.procs {
		if mp.cmd != nil && mp.cmd.Process != nil {
			_ = mp.cmd.Process.Kill()
		}
	}
	l.mu.Unlock()
	l.wg.Wait()
}

// supervise runs the session in a loop, restarting on unexpected exit with backoff.
func (l *SessionLauncher) supervise(s SessionConfig) {
	defer l.wg.Done()

	backoff := 2 * time.Second
	const maxBackoff = 60 * time.Second

	for {
		select {
		case <-l.stop:
			return
		default:
		}

		cmd := l.buildCmd(s)
		l.mu.Lock()
		l.procs[s.Name] = &managedProc{cfg: s, cmd: cmd}
		l.mu.Unlock()

		log.Printf("[launcher] starting session %q in %s", s.Name, s.Dir)
		if err := cmd.Start(); err != nil {
			log.Printf("[launcher] session %q failed to start: %v", s.Name, err)
		} else {
			_ = cmd.Wait()
		}

		select {
		case <-l.stop:
			return
		default:
		}

		log.Printf("[launcher] session %q exited — restarting in %s", s.Name, backoff)
		select {
		case <-l.stop:
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, maxBackoff)
	}
}

func (l *SessionLauncher) buildCmd(s SessionConfig) *exec.Cmd {
	args := []string{
		"--attach", l.serverURL,
		"--name", s.Name,
		"--headless",
	}
	if s.Master {
		args = append(args, "--master")
	}
	if s.Agent != "" {
		args = append(args, "--agent", s.Agent)
	}
	if s.Team != "" {
		args = append(args, "--team", s.Team)
	}

	cmd := exec.Command("claudio", args...)
	cmd.Dir = s.Dir

	// Forward env; inject password so claudio can authenticate with the hub.
	env := os.Environ()
	env = append(env, "COMANDCENTER_PASSWORD="+l.password)
	cmd.Env = env

	// Prefix log lines with the session name.
	cmd.Stdout = &prefixWriter{prefix: "[" + s.Name + "] "}
	cmd.Stderr = &prefixWriter{prefix: "[" + s.Name + "] "}

	return cmd
}

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

// prefixWriter prepends a fixed string to each line written to os.Stderr/Stdout.
type prefixWriter struct {
	prefix string
	buf    []byte
}

func (w *prefixWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		idx := -1
		for i, b := range w.buf {
			if b == '\n' {
				idx = i
				break
			}
		}
		if idx < 0 {
			break
		}
		line := w.buf[:idx+1]
		log.Printf("%s%s", w.prefix, line[:len(line)-1])
		w.buf = w.buf[idx+1:]
	}
	return len(p), nil
}

// defaultCCConfigPath returns ~/.claudio/cc-config.json.
func defaultCCConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "cc-config.json"
	}
	return filepath.Join(home, ".claudio", "cc-config.json")
}
