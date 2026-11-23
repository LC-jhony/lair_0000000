package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lair-sh/lair/internal/config"
)

// LaravelArgs builds the exact non-interactive arguments for `laravel new`.
// Credentials are deliberately absent: they travel through the environment.
func LaravelArgs(r CreateRequest) []string {
	args := []string{"new", r.Name}
	switch r.Stack {
	case "react", "vue", "svelte", "livewire":
		args = append(args, "--"+r.Stack)
	}
	if !r.Auth {
		args = append(args, "--no-authentication")
	}
	if r.Testing == "pest" {
		args = append(args, "--pest")
	} else {
		args = append(args, "--phpunit")
	}
	if r.Boost {
		args = append(args, "--boost")
	} else {
		args = append(args, "--no-boost")
	}
	switch r.NodeManager {
	case "pnpm", "bun", "yarn":
		args = append(args, "--"+r.NodeManager)
	case "none":
		args = append(args, "--no-node")
	default:
		args = append(args, "--npm")
	}
	if r.Git {
		args = append(args, "--git")
	}
	if r.Database != "" {
		args = append(args, "--database="+r.Database)
	}
	return append(args, "--no-interaction", "--no-ansi")
}

// CommandLine renders a password-free command for the job log.
func CommandLine(bin string, r CreateRequest) string {
	parts := append([]string{bin}, LaravelArgs(r)...)
	return strings.Join(parts, " ")
}

// EnvValues builds DB_* and agent-sanitised environment variables. The
// password is present here only: never in argv and never in logs.
func EnvValues(r CreateRequest, parent string) []string {
	database := r.DBName
	if r.Database == "sqlite" {
		database = filepath.Join(parent, r.Name, "database", "database.sqlite")
	}
	env := []string{
		"DB_CONNECTION=" + r.Database,
		"DB_HOST=" + r.DBHost,
		"DB_PORT=" + strconv.Itoa(r.DBPort),
		"DB_DATABASE=" + database,
		"DB_USERNAME=" + r.DBUser,
		"DB_PASSWORD=" + r.DBPassword,
	}
	if r.Database == "sqlite" {
		env[1] = "DB_HOST=localhost"
	}
	return env
}

// CleanEnv removes variables that make the Laravel installer switch to
// machine-readable output, then applies overrides. Existing PATH entries are
// preserved; PATH additions for Lair's shims are passed separately by caller.
func CleanEnv(base []string, overrides ...string) []string {
	blocked := map[string]bool{
		"OPENCODE_CLIENT": true, "OPENCODE": true, "CLAUDECODE": true,
		"CLAUDE_CODE": true, "CODEX": true, "AI_AGENT": true,
	}
	out := make([]string, 0, len(base)+len(overrides))
	for _, item := range base {
		parts := strings.SplitN(item, "=", 2)
		if blocked[strings.ToUpper(parts[0])] {
			continue
		}
		out = append(out, item)
	}
	return append(out, overrides...)
}

// Redact removes the database password from arbitrary command output before it
// reaches the job log.
func Redact(text, password string) string {
	if strings.TrimSpace(password) == "" {
		return text
	}
	return strings.ReplaceAll(text, password, "***")
}

// AppURL computes the APP_URL for the new project from Lair's web settings.
func AppURL(name string, domain string, web config.WebSettings, secure bool) string {
	host := strings.TrimSuffix(name, ".") + "." + strings.TrimPrefix(domain, ".")
	port := web.HTTPPort
	if secure && web.HTTPS {
		port = web.HTTPSPort
	}
	scheme := URLScheme(secure, web.HTTPS)
	if port == 80 || port == 443 {
		return fmt.Sprintf("%s://%s", scheme, host)
	}
	return fmt.Sprintf("%s://%s:%d", scheme, host, port)
}

// PatchEnvFile rewrites selected KEY=value lines in place, appending missing
// keys at the end. Comments and unrelated variables are preserved.
func PatchEnvFile(path string, values map[string]string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	seen := map[string]bool{}
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || !strings.Contains(line, "=") {
			continue
		}
		key := strings.TrimSpace(strings.SplitN(line, "=", 2)[0])
		value, ok := values[key]
		if !ok {
			continue
		}
		lines[i] = key + "=" + value
		seen[key] = true
	}
	for _, key := range sortedKeys(values) {
		if !seen[key] {
			lines = append(lines, key+"="+values[key])
		}
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

// EnvFileValues returns the values written to .env and .env.example. The
// example file never receives the real password.
func EnvFileValues(r CreateRequest, appURL, parent string, example bool) map[string]string {
	values := map[string]string{"APP_URL": appURL}
	if r.Database == "sqlite" {
		values["DB_CONNECTION"] = "sqlite"
		// Laravel requires an absolute path for SQLite; a relative one makes
		// every request fail with "Database file does not exist".
		values["DB_DATABASE"] = filepath.Join(parent, r.Name, "database", "database.sqlite")
		return values
	}
	values["DB_CONNECTION"] = r.Database
	values["DB_HOST"] = r.DBHost
	values["DB_PORT"] = strconv.Itoa(r.DBPort)
	values["DB_DATABASE"] = r.DBName
	values["DB_USERNAME"] = r.DBUser
	if example {
		values["DB_PASSWORD"] = ""
	} else {
		values["DB_PASSWORD"] = r.DBPassword
	}
	return values
}
