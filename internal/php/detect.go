package php

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/lair-sh/lair/internal/version"
)

// detectRE extracts a major.minor[.patch] token from a Composer constraint.
var detectRE = regexp.MustCompile(`(\d+)\.(\d+)(?:\.(\d+))?`)

type composerDoc struct {
	Require struct {
		PHP string `json:"php"`
	} `json:"require"`
}

// DetectConstraint returns the PHP requirement declared by a project, in this
// order of preference: .php-version, composer.json require.php. An empty
// string means the project declares nothing.
func DetectConstraint(dir string) (string, error) {
	if data, err := os.ReadFile(filepath.Join(dir, ".php-version")); err == nil {
		if c := strings.TrimSpace(string(data)); c != "" {
			return c, nil
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var doc composerDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return "", fmt.Errorf("parse composer.json: %w", err)
	}
	return strings.TrimSpace(doc.Require.PHP), nil
}

// Detect chooses the newest installed minor that satisfies the project's PHP
// constraint. The matcher is deliberately conservative: it understands the
// common Composer forms (^, >=, >, ~, exact, wildcards and || alternatives)
// and refuses a guess when a clause is not understood, rather than silently
// selecting an incompatible runtime. Framework-specific clamping can be
// applied by callers on top of the result.
func Detect(dir string, installed []Info) (minor string, constraint string, err error) {
	constraint, err = DetectConstraint(dir)
	if err != nil {
		return "", "", err
	}
	if constraint == "" {
		return "", "", nil
	}
	clauses := strings.FieldsFunc(constraint, func(r rune) bool { return r == '|' })
	satisfies := func(v version.Version) bool {
		for _, clause := range clauses {
			c := strings.TrimSpace(clause)
			if c == "" || c == "*" {
				return true
			}
			op := ""
			switch {
			case strings.HasPrefix(c, ">="):
				op, c = ">=", strings.TrimSpace(strings.TrimPrefix(c, ">="))
			case strings.HasPrefix(c, "<="):
				op, c = "<=", strings.TrimSpace(strings.TrimPrefix(c, "<="))
			case strings.HasPrefix(c, ">"), strings.HasPrefix(c, "<"):
				op, c = c[:1], strings.TrimSpace(c[1:])
			case strings.HasPrefix(c, "^"), strings.HasPrefix(c, "~"):
				op, c = c[:1], strings.TrimSpace(c[1:])
			}
			m := detectRE.FindStringSubmatch(c)
			if len(m) < 3 {
				// An unparsed clause must not produce a false positive.
				continue
			}
			major, _ := strconv.Atoi(m[1])
			minorN, _ := strconv.Atoi(m[2])
			lo := version.Version{Major: major, Minor: minorN}
			switch op {
			case "^", "~":
				if v.Major == lo.Major && v.Minor >= lo.Minor {
					return true
				}
			case ">=", ">":
				if v.Compare(lo) > 0 || (op == ">=" && v.Compare(lo) == 0) {
					return true
				}
			case "<=", "<":
				if v.Compare(lo) < 0 || (op == "<=" && v.Compare(lo) == 0) {
					return true
				}
			default:
				if v.Major == lo.Major && v.Minor == lo.Minor {
					return true
				}
			}
		}
		return false
	}

	candidates := make([]version.Version, 0, len(installed))
	for _, info := range installed {
		v, err := version.Parse(info.Minor)
		if err == nil && satisfies(v) {
			candidates = append(candidates, v)
		}
	}
	if len(candidates) == 0 {
		return "", constraint, fmt.Errorf("ninguna versión instalada satisface %q", constraint)
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Compare(candidates[j]) > 0 })
	return candidates[0].MinorLine(), constraint, nil
}
