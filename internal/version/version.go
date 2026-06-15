// Package version holds the release-version helper used by build tooling.
package version

import (
	"fmt"
	"strconv"
	"strings"
)

// GetNextVersion computes the next version from the latest git tag, mirroring
// the bump logic exercised by the original test suite. An empty tag yields the
// initial "1.0.0"; otherwise the minor component is incremented and the patch
// reset to 0.
func GetNextVersion(latestTag string) string {
	if latestTag == "" {
		return "1.0.0"
	}

	current := strings.TrimPrefix(latestTag, "v")
	parts := strings.Split(current, ".")
	if len(parts) != 3 {
		return "1.0.0"
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		major = 1
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		minor = 0
	}

	return fmt.Sprintf("%d.%d.0", major, minor+1)
}
