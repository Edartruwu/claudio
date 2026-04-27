package refresh

import (
	"fmt"
	"math/rand"
	"path/filepath"
	"sync"
	"time"

	"github.com/Abraxas-365/claudio/internal/auth/oauth"
	"github.com/Abraxas-365/claudio/internal/auth/storage"
	"github.com/Abraxas-365/claudio/internal/config"
	"github.com/gofrs/flock"
)

const (
	refreshBuffer   = 5 * time.Minute
	maxRetries      = 5
	lockRetryBase   = 1000 // ms
	lockRetryJitter = 1000 // ms
)

// in-process deduplication: only one goroutine runs a refresh at a time;
// others wait and share the result.
var (
	refreshMu       sync.Mutex
	refreshInFlight bool
	refreshCond     = sync.NewCond(&refreshMu)
	lastResult      bool
	lastErr         error
)

// CheckAndRefreshIfNeeded checks if the OAuth token needs refreshing and refreshes it.
// Uses in-process deduplication (only one goroutine refreshes at a time) and
// file locking for multi-process safety.
// profile scopes the lock file; empty string uses the legacy lock path.
func CheckAndRefreshIfNeeded(store storage.SecureStorage, force bool, profile ...string) (bool, error) {
	refreshMu.Lock()
	// If a refresh is already in flight and this is not a forced call, wait for it.
	if refreshInFlight && !force {
		for refreshInFlight {
			refreshCond.Wait()
		}
		result, err := lastResult, lastErr
		refreshMu.Unlock()
		return result, err
	}
	refreshInFlight = true
	refreshMu.Unlock()

	defer func() {
		refreshMu.Lock()
		refreshInFlight = false
		refreshCond.Broadcast()
		refreshMu.Unlock()
	}()

	profileName := ""
	if len(profile) > 0 {
		profileName = profile[0]
	}
	result, err := checkAndRefreshImpl(store, force, profileName)

	refreshMu.Lock()
	lastResult, lastErr = result, err
	refreshMu.Unlock()

	return result, err
}

func checkAndRefreshImpl(store storage.SecureStorage, force bool, profile string) (bool, error) {
	tokens, err := store.ReadTokens()
	if err != nil || tokens == nil || tokens.RefreshToken == "" {
		return false, nil
	}

	if !force && !tokens.IsExpired(refreshBuffer) {
		return false, nil
	}

	return refreshWithLock(store, tokens, 0, profile)
}

func refreshWithLock(store storage.SecureStorage, tokens *oauth.Tokens, retryCount int, profile string) (bool, error) {
	lockFile := ".oauth-refresh.lock"
	if profile != "" && profile != "default" {
		lockFile = fmt.Sprintf(".oauth-refresh-%s.lock", profile)
	}
	lockPath := filepath.Join(config.GetPaths().Home, lockFile)
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
		return refreshWithLock(store, tokens, retryCount+1, profile)
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
