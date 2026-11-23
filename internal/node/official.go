package node

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/lair-sh/lair/internal/manifest"
)

// OfficialBaseURL is the root of the official Node.js distribution. It is a
// variable so tests can point it at a local server.
var OfficialBaseURL = "https://nodejs.org/dist/"

var officialVersion = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:-[0-9A-Za-z.-]+)?$`)

// NormalizeOfficialVersion validates a free-form Node version and returns the
// canonical directory name used by nodejs.org (with a leading "v"). Partial
// requests such as "22" are rejected on purpose: installing an arbitrary
// version must be explicit.
func NormalizeOfficialVersion(request string) (string, error) {
	s := strings.TrimSpace(request)
	if !officialVersion.MatchString(s) {
		return "", fmt.Errorf("invalid Node version %q; use an exact version like 22.16.0", request)
	}
	if !strings.HasPrefix(s, "v") {
		s = "v" + s
	}
	return s, nil
}

func officialPlatform(platform string) (string, error) {
	switch platform {
	case "linux-x64", "linux-arm64":
		return platform, nil
	case "macos-x64":
		return "darwin-x64", nil
	case "macos-arm64":
		return "darwin-arm64", nil
	case "windows-x64":
		return "win-x64", nil
	case "windows-arm64":
		return "win-arm64", nil
	default:
		return "", fmt.Errorf("no official Node build for platform %q", platform)
	}
}

// OfficialArtifact resolves an exact version against the checksums published
// by nodejs.org. The signed catalog stays the source of truth for curated
// versions; this is the verified fallback for versions typed by the user.
func OfficialArtifact(ctx context.Context, client *http.Client, request, platform string) (manifest.Artifact, error) {
	ver, err := NormalizeOfficialVersion(request)
	if err != nil {
		return manifest.Artifact{}, err
	}
	nodePlatform, err := officialPlatform(platform)
	if err != nil {
		return manifest.Artifact{}, err
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	base := strings.TrimRight(OfficialBaseURL, "/") + "/"
	archive := fmt.Sprintf("node-%s-%s.tar.gz", ver, nodePlatform)
	sumURL := base + ver + "/SHASUMS256.txt"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sumURL, nil)
	if err != nil {
		return manifest.Artifact{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return manifest.Artifact{}, fmt.Errorf("fetch %s: %w", sumURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return manifest.Artifact{}, fmt.Errorf("%s returned %s", sumURL, resp.Status)
	}

	sha := ""
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && fields[1] == archive {
			sha = fields[0]
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return manifest.Artifact{}, err
	}
	if len(sha) != 64 {
		return manifest.Artifact{}, fmt.Errorf("no SHA-256 for %s in %s", archive, sumURL)
	}

	return manifest.Artifact{
		Build:      "official",
		URL:        base + ver + "/" + archive,
		SHA256:     sha,
		Published:  time.Now().UTC(),
		Prerelease: strings.Contains(ver, "-"),
	}, nil
}
