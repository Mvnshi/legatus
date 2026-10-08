package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Mvnshi/legatus/internal/store"
)

// DefaultAddr is where the daemon listens unless told otherwise.
const DefaultAddr = "127.0.0.1:7420"

var tokenPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// LoadOrCreateToken returns the secret that API calls must carry, creating it on first use. The file is
// readable only by the user.
func LoadOrCreateToken(root string) (string, error) {
	path := filepath.Join(root, "token")
	if data, err := os.ReadFile(path); err == nil {
		if t := strings.TrimSpace(string(data)); tokenPattern.MatchString(t) {
			return t, nil
		}
		return "", fmt.Errorf("%s does not hold a valid token; delete it to make a new one", path)
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	if err := os.WriteFile(path, []byte(token), 0o600); err != nil {
		return "", err
	}
	return token, nil
}

// DaemonInfo says where a running daemon can be reached.
type DaemonInfo struct {
	Addr    string    `json:"addr"`
	PID     int       `json:"pid"`
	Started time.Time `json:"started"`
}

func daemonFile(root string) string { return filepath.Join(root, "daemon.json") }

// ReadDaemon returns the recorded daemon, if one is recorded.
func ReadDaemon(root string) (*DaemonInfo, error) {
	data, err := os.ReadFile(daemonFile(root))
	if err != nil {
		return nil, err
	}
	var d DaemonInfo
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// Ping reports whether a daemon answers at addr with this token.
func Ping(addr, token string) bool {
	req, err := http.NewRequest("GET", "http://"+addr+"/api/health", nil)
	if err != nil {
		return false
	}
	req.Header.Set("X-Legatus-Token", token)
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

// Serve listens on addr and answers until ctx ends. It records itself in daemon.json so the command line
// can find it, and refuses to start when another daemon already answers for this directory.
func (s *Server) Serve(ctx context.Context, addr string, ready func(net.Addr)) error {
	root := s.App.Root
	if d, err := ReadDaemon(root); err == nil && Ping(d.Addr, s.Token) {
		return fmt.Errorf("a Legatus daemon is already running for these files (%s, pid %d)", d.Addr, d.PID)
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("%q is not a host:port address", addr)
	}
	if !loopbackHost(host) {
		return errors.New("Legatus only listens on this computer (127.0.0.1 or localhost); it has no login or encryption for anything wider yet")
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("cannot listen on %s: %w (is another program using it? try --addr 127.0.0.1:0 for any free port)", addr, err)
	}
	info := DaemonInfo{Addr: ln.Addr().String(), PID: os.Getpid(), Started: time.Now().UTC()}
	data, _ := json.Marshal(info)
	tmp := daemonFile(root) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		ln.Close()
		return err
	}
	if err := store.RenameReplace(tmp, daemonFile(root)); err != nil {
		ln.Close()
		return err
	}
	defer os.Remove(daemonFile(root))

	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	if ready != nil {
		ready(ln.Addr())
	}
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdown)
	srv.Close() // streams that are still open do not hold up shutdown
	return nil
}
