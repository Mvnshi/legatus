// Package app wires the parts together for one Legatus installation: where its files live, the saved
// accounts and their limits, the agent backends, and the engine.
package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Mvnshi/legatus/internal/agent"
	"github.com/Mvnshi/legatus/internal/agent/claude"
	"github.com/Mvnshi/legatus/internal/agent/codex"
	"github.com/Mvnshi/legatus/internal/engine"
	"github.com/Mvnshi/legatus/internal/pool"
	"github.com/Mvnshi/legatus/internal/store"
	"github.com/Mvnshi/legatus/internal/worktree"
)

// Root is the directory Legatus keeps everything in: $LEGATUS_HOME, or the user's config directory.
func Root() (string, error) {
	if v := os.Getenv("LEGATUS_HOME"); v != "" {
		return filepath.Abs(v)
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("cannot find a configuration directory (set LEGATUS_HOME): %w", err)
	}
	return filepath.Join(dir, "legatus"), nil
}

// App is one open installation.
type App struct {
	Root   string
	Store  *store.Store
	Pool   *pool.Pool
	Engine *engine.Engine
}

// Options customise Open. Backends replaces the real agents (tests and the demo use this).
type Options struct {
	Backends map[string]agent.Backend
}

// Open creates the directory layout if needed and loads the saved accounts.
func Open(root string, opts Options) (*App, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	st, err := store.Open(root)
	if err != nil {
		return nil, err
	}
	a := &App{Root: root, Store: st, Pool: pool.New()}
	if err := a.loadAccounts(); err != nil {
		return nil, err
	}
	a.Pool.OnChange = func() { _ = a.SaveAccounts() }
	backends := opts.Backends
	if backends == nil {
		backends = map[string]agent.Backend{
			"codex":  &codex.Backend{},
			"claude": &claude.Backend{},
		}
	}
	a.Engine = &engine.Engine{
		Store: st, Pool: a.Pool, Worktrees: worktree.New(), Backends: backends, DataDir: root,
	}
	return a, nil
}

func (a *App) accountsFile() string { return filepath.Join(a.Root, "accounts.json") }

func (a *App) loadAccounts() error {
	data, err := os.ReadFile(a.accountsFile())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var snaps []pool.Snapshot
	if err := json.Unmarshal(data, &snaps); err != nil {
		return fmt.Errorf("%s is not valid: %w", a.accountsFile(), err)
	}
	a.Pool.Restore(snaps)
	return nil
}

// SaveAccounts writes the accounts and their current limits.
func (a *App) SaveAccounts() error {
	data, err := json.MarshalIndent(a.Pool.Snapshots(), "", "  ")
	if err != nil {
		return err
	}
	tmp := a.accountsFile() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, a.accountsFile())
}

// AccountHome is the default configuration directory for an account created by Legatus.
func (a *App) AccountHome(provider, id string) string {
	return filepath.Join(a.Root, "accounts", provider+"-"+id)
}
