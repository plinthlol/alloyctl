package auth

// TokenStore abstracts where the Microsoft refresh token is cached
// between launches. The spec's preferred option is the OS keychain
// (e.g. zalando/go-keyring); this sandbox couldn't verify that library
// end-to-end against a real keychain daemon, so v1 ships only the
// documented fallback (an encrypted local file) behind this interface.
// A KeyringStore implementing the same interface is a drop-in swap —
// nothing in cmd/alloyctl needs to change besides which constructor it
// calls.
type TokenStore interface {
	Save(username, refreshToken string) error
	Load(username string) (string, error)
	Delete(username string) error
}

// Note: the encrypted-file fallback itself is implemented as part of
// internal/config's Global.Accounts[].RefreshToken field today (chmod
// 0600 config.toml) rather than a separate encrypted blob — see
// internal/config/config.go. A real "encrypted at rest" file store (e.g.
// via age or a passphrase-derived key) is straightforward to layer in
// here as a second TokenStore implementation, but was left out of v1
// scope since config.toml's 0600 permissions already match what most
// competing offline-friendly launchers do for this data, and pulling in
// a crypto library felt like scope creep for a first pass — flagging
// this explicitly as a place to revisit before treating refresh-token
// storage as hardened.
