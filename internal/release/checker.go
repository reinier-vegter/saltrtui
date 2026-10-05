// Package release checks GitHub for newer saltrtui releases and performs
// confirmed, verified self-updates of the Linux amd64 standalone binary.
package release

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"saltrtui/internal/cache"
)

const latestReleaseURL = "https://api.github.com/repos/reinier-vegter/saltrtui/releases/latest"

// Result describes the latest release known to be newer than the running binary.
type Result struct {
	LatestVersion string
	Available     bool
}

// CheckLatest returns a cached result when it was checked within the last hour.
func CheckLatest(ctx context.Context, currentVersion string, store *cache.Store) (Result, error) {
	return checkLatest(ctx, currentVersion, store, http.DefaultClient, time.Now, latestReleaseURL)
}

func checkLatest(ctx context.Context, currentVersion string, store *cache.Store, client *http.Client, now func() time.Time, url string) (Result, error) {
	if !isReleaseVersion(currentVersion) {
		return Result{}, nil
	}

	checkedAt := now()
	check, err := store.LoadUpdateCheck()
	if err == nil && cache.IsUpdateCheckFresh(check, checkedAt) {
		return resultFor(currentVersion, check.LatestVersion), nil
	}

	latest := check.LatestVersion
	fetchedLatest, fetchErr := fetchLatest(ctx, client, url)
	if fetchErr == nil {
		latest = fetchedLatest
	}
	// Persist the attempt even when GitHub cannot be reached to enforce the rate limit.
	if saveErr := store.SaveUpdateCheck(cache.UpdateCheck{CheckedAt: checkedAt, LatestVersion: latest}); saveErr != nil {
		return Result{}, fmt.Errorf("save update check: %w", saveErr)
	}
	return resultFor(currentVersion, latest), nil
}

func fetchLatest(ctx context.Context, client *http.Client, url string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "saltrtui-update-check")
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("request latest release: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("request latest release: GitHub returned %s", response.Status)
	}
	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode latest release: %w", err)
	}
	if !isReleaseVersion(payload.TagName) {
		return "", fmt.Errorf("latest release has invalid tag %q", payload.TagName)
	}
	return payload.TagName, nil
}

func resultFor(currentVersion, latestVersion string) Result {
	return Result{LatestVersion: latestVersion, Available: isNewer(latestVersion, currentVersion)}
}

func isReleaseVersion(version string) bool {
	_, ok := parseVersion(version)
	return ok
}

func isNewer(candidate, current string) bool {
	candidateParts, candidateOK := parseVersion(candidate)
	currentParts, currentOK := parseVersion(current)
	if !candidateOK || !currentOK {
		return false
	}
	for index := range candidateParts {
		if candidateParts[index] != currentParts[index] {
			return candidateParts[index] > currentParts[index]
		}
	}
	return false
}

func parseVersion(version string) ([3]int, bool) {
	var result [3]int
	parts := strings.Split(strings.TrimPrefix(version, "v"), ".")
	if len(parts) != len(result) {
		return result, false
	}
	for index, part := range parts {
		if part == "" {
			return result, false
		}
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			return result, false
		}
		result[index] = value
	}
	return result, true
}
