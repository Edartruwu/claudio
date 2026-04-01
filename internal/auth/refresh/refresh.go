package refresh

import (
	"fmt"
	"math/rand"
	"path/filepath"
	"time"

	"github.com/Abraxas-365/claudio/internal/auth/oauth"
	"github.com/Abraxas-365/claudio/internal/auth/storage"
	"github.com/Abraxas-365/claudio/internal/config"
	"github.com/gofrs/flock"
)

const (
	refreshBuffer  = 5 * time.Minute
	maxRetries     = 5
	lockRetryBase  = 1000 // ms
	lockRetryJitter = 1000 // ms
)

// CheckAndRefreshIfNeeded checks if the OAuth token needs refreshing and refreshes it.
// Uses file locking for multi-process safety.
func CheckAndRefreshIfNeeded(store storage.SecureStorage, force bool) (refreshed bool, err error) {
	tokens, err := store.ReadTokens()
	if err != nil || tokens == nil || tokens.RefreshToken == "" {
		return false, nil
	}

	if !force && !tokens.IsExpired(refreshBuffer) {
		return false, nil
	}

	return refreshWithLock(store, tokens, 0)
}

func refreshWithLock(store storage.SecureStorage, tokens *oauth.Tokens, retryCount int) (bool, error) {
	lockPath := filepath.Join(config.GetPaths().Home, ".oauth-refresh.lock")
	fileLock := flock.New(lockPath)

	locked, err := fileLock.TryLock()
	if err != nil {
		return false, fmt.Errorf("failed to acquire refresh lock: %w", err)
	}

	if !locked {
		// Another process holds the lock
		if retryCount >= maxRetries {
			return false, fmt.Errorf("could not acquire refresh lock after %d retries", maxRetries)
		}
		wait := time.Duration(lockRetryBase+rand.Intn(lockRetryJitter)) * time.Millisecond
		time.Sleep(wait)

		// Re-read tokens — other process may have refreshed
		tokens, err = store.ReadTokens()
		if err != nil {
			return false, err
		}
		if tokens != nil && !tokens.IsExpired(refreshBuffer) {
			return false, nil // Race resolved
		}
		return refreshWithLock(store, tokens, retryCount+1)
	}
	defer fileLock.Unlock()

	// Double-check after acquiring lock (another process may have refreshed)
	tokens, err = store.ReadTokens()
	if err != nil {
		return false, err
	}
	if tokens == nil || tokens.RefreshToken == "" {
		return false, nil
	}
	if !tokens.IsExpired(refreshBuffer) {
		return false, nil // Race resolved
	}

	// Perform the refresh
	client := oauth.NewClient(oauth.DefaultConfig())
	newTokens, err := client.RefreshToken(tokens.RefreshToken, tokens.Scopes)
	if err != nil {
		return false, fmt.Errorf("token refresh failed: %w", err)
	}

	// Preserve fields not returned by refresh
	if newTokens.Account == nil {
		newTokens.Account = tokens.Account
	}
	if newTokens.Profile == nil {
		newTokens.Profile = tokens.Profile
	}
	if newTokens.SubscriptionType == "" {
		newTokens.SubscriptionType = tokens.SubscriptionType
	}
	if newTokens.RateLimitTier == "" {
		newTokens.RateLimitTier = tokens.RateLimitTier
	}

	if err := store.SaveTokens(newTokens); err != nil {
		return false, fmt.Errorf("failed to save refreshed tokens: %w", err)
	}

	return true, nil
}
