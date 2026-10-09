// Package app wires the parts together for one Legatus installation: where its files live, the saved
// accounts and their limits, the agent backends, and the engine.
package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

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
	return store.RenameReplace(tmp, a.accountsFile())
}

// AccountHome is the default configuration directory for an account created by Legatus.
func (a *App) AccountHome(provider, id string) string {
	return filepath.Join(a.Root, "accounts", provider+"-"+id)
}

var accountID = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// ValidAccountID reports whether name can be used as an account id.
func ValidAccountID(name string) bool { return accountID.MatchString(name) }

// AddAccount registers a login. home is "" for a new folder Legatus creates, "default" for the login the
// agent already has on this computer, or the path of an existing folder.
func (a *App) AddAccount(id, provider, home, label string, max int) (pool.Account, error) {
	if !ValidAccountID(id) {
		return pool.Account{}, errors.New("the account name must be 1-32 lowercase letters, digits, - or _")
	}
	if provider != "codex" && provider != "claude" {
		return pool.Account{}, errors.New("the provider must be codex or claude")
	}
	if max < 1 {
		return pool.Account{}, errors.New("an account must allow at least 1 task at a time")
	}
	for _, s := range a.Pool.Snapshots() {
		if s.ID == id {
			return pool.Account{}, fmt.Errorf("an account named %q already exists", id)
		}
	}
	dir := home
	switch home {
	case "":
		dir = a.AccountHome(provider, id)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return pool.Account{}, err
		}
	case "default":
		dir = ""
	default:
		if err := os.MkdirAll(home, 0o700); err != nil {
			return pool.Account{}, err
		}
	}
	acct := pool.Account{ID: id, Provider: provider, Label: label, Home: dir, MaxConcurrent: max}
	if err := a.Pool.Add(acct); err != nil {
		return pool.Account{}, err
	}
	return acct, a.SaveAccounts()
}

// RemoveAccount forgets a login. Its folder is left alone.
func (a *App) RemoveAccount(id string) error {
	for _, s := range a.Pool.Snapshots() {
		if s.ID == id {
			a.Pool.Remove(id)
			return a.SaveAccounts()
		}
	}
	return fmt.Errorf("no account named %q", id)
}

// SetAccountDisabled turns a login off or on.
func (a *App) SetAccountDisabled(id string, disabled bool) error {
	if !a.Pool.SetDisabled(id, disabled) {
		return fmt.Errorf("no account named %q", id)
	}
	return a.SaveAccounts()
}

var optionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/+-]{0,63}$`)

// SetAccountOptions changes which model a login asks its agent for, and how hard it thinks. An empty value
// puts the agent's own default back.
func (a *App) SetAccountOptions(id, model, effort string) error {
	for _, v := range []string{model, effort} {
		if v != "" && !optionPattern.MatchString(v) {
			return fmt.Errorf("%q is not a model or effort name", v)
		}
	}
	for _, s := range a.Pool.Snapshots() {
		if s.ID == id {
			acct := s.Account
			acct.Model, acct.Effort = model, effort
			if err := a.Pool.Add(acct); err != nil {
				return err
			}
			return a.SaveAccounts()
		}
	}
	return fmt.Errorf("no account named %q", id)
}
