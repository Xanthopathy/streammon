package updatecheck

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type gitHubReleaseInfo struct {
	TagName string `json:"tag_name"`
}

// CheckForUpdates checks the latest GitHub release and compares it to the current version.
// It returns a formatted message if a new version is available, or an empty string otherwise.
// It performs a simple string comparison and does not handle complex semantic versioning.
func CheckForUpdates(currentVersion string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	url := "https://api.github.com/repos/Xanthopathy/streammon/releases/latest"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bad status from GitHub API: %s", resp.Status)
	}

	var releaseInfo gitHubReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&releaseInfo); err != nil {
		return "", fmt.Errorf("failed to decode release info: %w", err)
	}

	latestVersion := releaseInfo.TagName

	if isNewerVersion(latestVersion, currentVersion) {
		releasesURL := "https://github.com/Xanthopathy/streammon/releases"
		return fmt.Sprintf("A new version is available: %s (current: %s). Get it here: %s", latestVersion, currentVersion, releasesURL), nil
	}

	return "", nil
}

// isNewerVersion compares two version strings (e.g. "v1.0.5" vs "v1.0.4").
// Returns true if remote is strictly greater than local.
func isNewerVersion(remote, local string) bool {
	parse := func(v string) []int {
		v = strings.TrimPrefix(strings.TrimSpace(v), "v")
		parts := strings.Split(v, ".")
		var nums []int
		for _, p := range parts {
			// Handle potential suffixes like -beta by ignoring them
			if idx := strings.IndexAny(p, "-+"); idx != -1 {
				p = p[:idx]
			}
			n, _ := strconv.Atoi(p)
			nums = append(nums, n)
		}
		return nums
	}

	rParts := parse(remote)
	lParts := parse(local)

	maxLen := len(rParts)
	if len(lParts) > maxLen {
		maxLen = len(lParts)
	}

	for i := 0; i < maxLen; i++ {
		r := 0
		if i < len(rParts) {
			r = rParts[i]
		}
		l := 0
		if i < len(lParts) {
			l = lParts[i]
		}

		if r > l {
			return true
		}
		if r < l {
			return false
		}
	}
	return false
}
