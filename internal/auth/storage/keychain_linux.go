//go:build linux

package storage

// NewKeychainStorage on Linux returns a plaintext storage fallback.
// TODO: Implement libsecret/kwallet support via go-keyring.
func NewKeychainStorage() *PlaintextStorage {
	return NewPlaintextStorage(defaultCredentialsPath())
}

func defaultCredentialsPath() string {
	return "" // Will be set by the composite storage
}
