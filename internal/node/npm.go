package node

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lair-sh/lair/internal/app"
	"github.com/lair-sh/lair/internal/version"
)

// NPMRegistryURL is the public npm registry metadata used by "Update npm".
// The registry is only read; the actual install is performed by npm itself
// into this major's isolated global prefix.
const NPMRegistryURL = "https://registry.npmjs.org/npm"

var engineMajor = regexp.MustCompile(`[0-9]+`)

type npmRegistry struct {
	DistTags map[string]string `json:"dist-tags"`
	Versions map[string]struct {
		Engines struct {
			Node string `json:"node"`
		} `json:"engines"`
	} `json:"versions"`
}

// LatestNPM returns the newest npm version whose declared engines.node range
// can run on the given Node major. Metadata is fetched from the public npm
// registry; no package is downloaded or installed here.
func LatestNPM(ctx context.Context, client *http.Client, nodeMajor string) (string, error) {
	major, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(nodeMajor), "v"))
	if err != nil {
		return "", fmt.Errorf("invalid Node major %q", nodeMajor)
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, NPMRegistryURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch npm registry: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("npm registry returned %s", resp.Status)
	}
	var doc npmRegistry
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return "", fmt.Errorf("decode npm registry: %w", err)
	}

	// Collect candidates: every published version, with dist-tags first so a
	// newly published compatible release is never missed.
	candidates := make([]string, 0, len(doc.Versions)+len(doc.DistTags))
	for name := range doc.Versions {
		candidates = append(candidates, name)
	}
	for _, name := range doc.DistTags {
		if _, ok := doc.Versions[name]; ok {
			continue
		}
		candidates = append(candidates, name)
	}
	compatible := make([]string, 0, len(candidates))
	for _, name := range candidates {
		meta, ok := doc.Versions[name]
		if !ok {
			continue
		}
		if _, err := version.Parse(name); err != nil {
			continue
		}
		if minMajor, ok := minimumEngineMajor(meta.Engines.Node); ok && major < minMajor {
			continue
		}
		compatible = append(compatible, name)
	}
	if len(compatible) == 0 {
		return "", fmt.Errorf("no npm release is compatible with Node %s", nodeMajor)
	}
	sort.Slice(compatible, func(i, j int) bool {
		a, ea := version.Parse(compatible[i])
		b, eb := version.Parse(compatible[j])
		if ea != nil || eb != nil {
			return compatible[i] > compatible[j]
		}
		return a.Compare(b) > 0
	})
	return compatible[0], nil
}

// minimumEngineMajor extracts the lowest Node major mentioned in an engines
// string. Ranges such as ">=18.17.0 || >=20.5.0" are satisfied by any branch,
// so the minimum of the alternatives is the correct check. Only the first
// number of each alternative is a major; patch numbers are ignored.
func minimumEngineMajor(engine string) (int, bool) {
	best := 0
	for _, alternative := range strings.Split(engine, "||") {
		match := engineMajor.FindString(alternative)
		if match == "" {
			continue
		}
		n, err := strconv.Atoi(match)
		if err != nil || n > 100 || n < 4 {
			continue
		}
		if best == 0 || n < best {
			best = n
		}
	}
	return best, best > 0
}

// UpdateNPM installs the newest npm compatible with this Node major into the
// isolated prefix. It returns the npm version that was installed.
func UpdateNPM(ctx context.Context, client *http.Client, paths app.Paths, versionDir, major string) (string, error) {
	target, err := LatestNPM(ctx, client, major)
	if err != nil {
		return "", err
	}
	name := "npm"
	if strings.HasPrefix(app.Platform(), "windows-") {
		name = "npm.cmd"
	}
	npmBin := filepath.Join(BinDir(versionDir), name)
	cmd := exec.CommandContext(ctx, npmBin, "install", "-g", "npm@"+target)
	// Last occurrence wins in exec.Cmd.Env; overrides go last.
	cmd.Env = append(os.Environ(), Env(paths, versionDir, major)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("npm install -g npm@%s: %w (%s)", target, err, strings.TrimSpace(string(out)))
	}
	return target, nil
}
