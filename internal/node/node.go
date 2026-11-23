// Package node manages official Node.js builds under node/<version>/ and the
// per-project version resolution used by the shims.
package node

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lair-sh/lair/internal/app"
	"github.com/lair-sh/lair/internal/version"
)

// ResolveVersion picks a Node version for a project. Precedence:
//
//	isolated > .lairrc (node=) > legacy .flockrc > .nvmrc/.node-version > default > newest
//
// Alias values such as lts/*, lts/hydrogen and "node" are ignored so they do
// not silently resolve to something unpredictable.
func ResolveVersion(projectDir, isolated, defaultVersion string, available []string) (string, error) {
	if isolated != "" {
		return version.ExactOrLatest(isolated, available)
	}
	// .lairrc is canonical; .flockrc is still read for existing projects.
	for _, name := range []string{".lairrc", ".flockrc"} {
		if v := fromRCFile(filepath.Join(projectDir, name)); v != "" {
			return version.ExactOrLatest(v, available)
		}
	}
	for _, name := range []string{".nvmrc", ".node-version"} {
		if v := fromVersionFile(filepath.Join(projectDir, name)); v != "" {
			return version.ExactOrLatest(v, available)
		}
	}
	if defaultVersion != "" {
		return version.ExactOrLatest(defaultVersion, available)
	}
	if len(available) == 0 {
		return "", fmt.Errorf("no Node versions installed")
	}
	version.SortNewest(available)
	return available[0], nil
}

func fromRCFile(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "node=") || strings.HasPrefix(line, "node:") {
			return cleanAlias(strings.TrimSpace(line[strings.IndexAny(line, "=:"):][1:]))
		}
	}
	return ""
}

func fromVersionFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return cleanAlias(strings.TrimSpace(string(data)))
}

func cleanAlias(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, "lts") || strings.Contains(s, "*") || s == "node" || s == "stable" {
		return ""
	}
	// Keep only a leading version-like token.
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	v := fields[0]
	if _, err := version.Parse(v); err != nil {
		return ""
	}
	return v
}

// GlobalPrefix returns the npm prefix for a major line. Global packages
// survive patch/minor upgrades inside the same major because the prefix path
// only depends on the major number.
func GlobalPrefix(paths app.Paths, major string) string {
	return filepath.Join(paths.Node, "global", major)
}

// CurrentLink is the stable path added to the shell PATH. It points at the
// prefix of the default major (symlink on Unix, junction on Windows).
func CurrentLink(paths app.Paths) string {
	return filepath.Join(paths.Node, "global", "current")
}

// EnsureGlobal creates the per-major npm prefix and updates the `current`
// link to it. npm installs -g into this prefix, so global CLIs survive an
// update inside the same major line.
func EnsureGlobal(paths app.Paths, major string) error {
	prefix := GlobalPrefix(paths, major)
	if err := os.MkdirAll(filepath.Join(prefix, "bin"), 0o755); err != nil {
		return err
	}
	return linkCurrent(prefix, CurrentLink(paths))
}

// BinDir is where node/npm/npx live inside an installed version. Official
// Windows zips keep the executables at the version root, tarballs use bin/.
func BinDir(versionDir string) string {
	if strings.HasPrefix(app.Platform(), "windows-") {
		return versionDir
	}
	return filepath.Join(versionDir, "bin")
}

// Binary is the absolute path of the node executable in an installed version.
func Binary(versionDir string) string {
	if strings.HasPrefix(app.Platform(), "windows-") {
		return filepath.Join(versionDir, "node.exe")
	}
	return filepath.Join(versionDir, "bin", "node")
}

// Expected lists the executables a Node archive must contain on this platform.
func Expected() []string {
	if strings.HasPrefix(app.Platform(), "windows-") {
		return []string{"node.exe", "npm.cmd", "npx.cmd"}
	}
	return []string{"bin/node", "bin/npm", "bin/npx"}
}

// Env returns environment variables that point npm at the per-major prefix
// and expose the local CA to Node TLS. The prefix's bin directory comes first
// so an updated npm and user-installed global CLIs win over Node's bundled
// versions.
func Env(paths app.Paths, versionDir, major string) []string {
	caFile := filepath.Join(paths.Certs, "ca.pem")
	prefix := GlobalPrefix(paths, major)
	path := filepath.Join(prefix, "bin") + string(os.PathListSeparator) +
		BinDir(versionDir) + string(os.PathListSeparator) + os.Getenv("PATH")
	return []string{
		"NODE_EXTRA_CA_CERTS=" + caFile,
		"NPM_CONFIG_PREFIX=" + prefix,
		"PATH=" + path,
	}
}

// RemoveOlderInMajor deletes older versions of the same major, keeping one
// version per major as required by the catalog. It never touches other majors
// or the global prefix directories.
func RemoveOlderInMajor(paths app.Paths, keep string) error {
	keepParsed, err := ParseMajor(keep)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(paths.Node)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "global" || entry.Name() == keep {
			continue
		}
		major, err := ParseMajor(entry.Name())
		if err != nil || major != keepParsed {
			continue
		}
		if err := os.RemoveAll(filepath.Join(paths.Node, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

// ParseMajor returns the major line of a version string.
func ParseMajor(v string) (string, error) { return MajorOf(v) }

// MajorOf returns the major line of a version string.
func MajorOf(v string) (string, error) {
	parsed, err := version.Parse(v)
	if err != nil {
		return "", err
	}
	return strconv.Itoa(parsed.Major), nil
}
