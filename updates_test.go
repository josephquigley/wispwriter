package writefreely

import (
	"regexp"
	"sync"
	"testing"
	"time"
)

func TestUpdatesRoundTrip(t *testing.T) {
	cache := newUpdatesCache(defaultUpdatesCacheTime)
	t.Run("New Updates Cache", func(t *testing.T) {

		if cache == nil {
			t.Fatal("Returned nil cache")
		}

		if cache.frequency != defaultUpdatesCacheTime {
			t.Fatalf("Got cache expiry frequency: %s but expected: %s", cache.frequency, defaultUpdatesCacheTime)
		}

		if cache.currentVersion != "v"+softwareVer {
			t.Fatalf("Got current version: %s but expected: %s", cache.currentVersion, "v"+softwareVer)
		}
	})

	t.Run("Release URL", func(t *testing.T) {
		url := cache.ReleaseNotesURL()

		reg, err := regexp.Compile(`^https:\/\/blog.writefreely.org\/version(-\d+){1,}$`)
		if err != nil {
			t.Fatalf("Test Case Error: Failed to compile regex: %v", err)
		}
		match := reg.MatchString(url)

		if !match {
			t.Fatalf("Malformed Release URL: %s", url)
		}
	})

	t.Run("Check Now", func(t *testing.T) {
		// ensure time between init and next check
		time.Sleep(1 * time.Second)

		prevLastCheck := cache.lastCheck

		// force to known older version for latest and current
		prevLatestVer := "v0.8.1"
		cache.latestVersion = prevLatestVer
		cache.currentVersion = "v0.8.0"

		err := cache.CheckNow()
		if err != nil {
			t.Fatalf("Error should be nil, got: %v", err)
		}

		if prevLastCheck == cache.lastCheck {
			t.Fatal("Expected lastCheck to update")
		}

		if cache.lastCheck.Before(prevLastCheck) {
			t.Fatal("Last check should be newer than previous")
		}

		if prevLatestVer == cache.latestVersion {
			t.Fatal("expected latestVersion to update")
		}

	})

	t.Run("Are Available", func(t *testing.T) {
		if !cache.AreAvailable() {
			t.Fatalf("Cache reports not updates but Current is %s and Latest is %s", cache.currentVersion, cache.latestVersion)
		}
	})

	t.Run("Latest Version", func(t *testing.T) {
		gotLatest := cache.LatestVersion()
		if gotLatest != cache.latestVersion {
			t.Fatalf("Malformed latest version. Expected: %s but got: %s", cache.latestVersion, gotLatest)
		}
	})
}

// TestUpdatesCacheConcurrentAccess covers the cache being read while the
// background check writes to it, which is what newUpdatesCache sets up on
// every start: it launches CheckNow in a goroutine and hands the cache
// straight to callers. Run with -race, this fails when the readers take a
// copy of the cache instead of locking it.
func TestUpdatesCacheConcurrentAccess(t *testing.T) {
	cache := &updatesCache{
		frequency:      time.Hour,
		currentVersion: "v0.1.0",
		lastCheck:      time.Now(),
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cache.AreAvailableNoCheck()
			cache.LatestVersion()
			cache.LastChecked()
			cache.CheckFailed()
			cache.ReleaseNotesURL()
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 8; i++ {
			cache.mu.Lock()
			cache.latestVersion = "v0.2.0"
			cache.lastCheck = time.Now()
			cache.mu.Unlock()
		}
	}()

	wg.Wait()
}
