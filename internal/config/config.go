package config

import (
	"os"

	"github.com/pelletier/go-toml/v2"
)

// Account is a stored login — either offline or Microsoft/online.
type Account struct {
	Type     string `toml:"type"` // "offline" or "microsoft"
	Username string `toml:"username"`
	UUID     string `toml:"uuid"`

	// Online-only fields. RefreshToken is stored here as a fallback when the
	// OS keychain isn't available; see internal/auth for details. In a real
	// deployment this file should be chmod 0600, which Save() enforces.
	RefreshToken string `toml:"refresh_token,omitempty"`
}

// Global is the top-level config.toml: accounts, java overrides, defaults.
type Global struct {
	// ActiveAccount is the username of the account used when none is given
	// explicitly.
	ActiveAccount string    `toml:"active_account,omitempty"`
	Accounts      []Account `toml:"accounts,omitempty"`

	// JavaPath, if set, overrides auto-detection globally. Per-instance
	// java_path (in instance.toml) always wins over this.
	JavaPath string `toml:"java_path,omitempty"`

	// DefaultMemoryMB is used for new instances that don't override it.
	DefaultMemoryMB int `toml:"default_memory_mb,omitempty"`
}

// DefaultGlobal returns sane defaults for a first run.
func DefaultGlobal() Global {
	return Global{DefaultMemoryMB: 2048}
}

// Load reads config.toml, returning defaults if it doesn't exist yet.
func Load(p Paths) (Global, error) {
	data, err := os.ReadFile(p.GlobalConfigFile())
	if os.IsNotExist(err) {
		return DefaultGlobal(), nil
	}
	if err != nil {
		return Global{}, err
	}
	var g Global
	if err := toml.Unmarshal(data, &g); err != nil {
		return Global{}, err
	}
	return g, nil
}

// Save writes config.toml, creating the config directory if needed.
func Save(p Paths, g Global) error {
	if err := os.MkdirAll(p.ConfigDir, 0o755); err != nil {
		return err
	}
	data, err := toml.Marshal(g)
	if err != nil {
		return err
	}
	return os.WriteFile(p.GlobalConfigFile(), data, 0o600)
}

// FindAccount returns the account with the given username, if any.
func (g Global) FindAccount(username string) (Account, bool) {
	for _, a := range g.Accounts {
		if a.Username == username {
			return a, true
		}
	}
	return Account{}, false
}

// UpsertAccount adds or replaces an account by username.
func (g *Global) UpsertAccount(a Account) {
	for i, existing := range g.Accounts {
		if existing.Username == a.Username {
			g.Accounts[i] = a
			return
		}
	}
	g.Accounts = append(g.Accounts, a)
}
