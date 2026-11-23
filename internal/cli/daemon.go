package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/lair-sh/lair/internal/app"
	"github.com/lair-sh/lair/internal/assets"
	"github.com/lair-sh/lair/internal/daemon"
	"github.com/lair-sh/lair/internal/dns"
	"github.com/lair-sh/lair/internal/manifest"
)

// runDaemon starts the long-running daemon. It uses the embedded public key
// and the same mirrors as the CLI, and honours LAIR_MANIFEST_BASE_URLS.
func runDaemon(args []string, out, errOut io.Writer) error {
	foreground := false
	initialPaths, err := mustPaths()
	if err != nil {
		return err
	}
	cfg := daemon.Defaults(initialPaths, nil)
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--foreground", "-f":
			foreground = true
		case "--no-web":
			cfg.EnableWeb = false
		case "--no-dns":
			cfg.EnableDNS = false
		case "--no-api":
			cfg.EnableAPI = false
		case "--no-ui", "--no-desktop":
			cfg.EnableDesktop = false
		case "--ui-port", "--desktop-port":
			cfg.DesktopPort, err = parseFlagInt(args, &i, args[i], 1, 65535)
			if err != nil {
				return err
			}
		case "--dns-port":
			cfg.DNSPort, err = parseFlagInt(args, &i, args[i], 1, 65535)
			if err != nil {
				return err
			}
		case "--http-port":
			cfg.HTTPPort, err = parseFlagInt(args, &i, args[i], 1, 65535)
			if err != nil {
				return err
			}
		case "--https-port":
			cfg.HTTPSPort, err = parseFlagInt(args, &i, args[i], 1, 65535)
			if err != nil {
				return err
			}
		case "--domain":
			if i+1 >= len(args) {
				return fmt.Errorf("--domain requires a value")
			}
			cfg.Domain = args[i+1]
			i++
		default:
			return fmt.Errorf("unknown daemon flag %q", args[i])
		}
	}
	if !foreground {
		fmt.Fprintln(out, "The daemon is managed by systemd. Use `sudo systemctl start lair@<user>` or `lair daemon --foreground`.")
		return nil
	}

	paths, err := mustPaths()
	if err != nil {
		return err
	}
	catalog, err := buildCatalog(paths)
	if err != nil {
		return err
	}
	cfg.Paths = paths
	cfg.Catalog = catalog
	if cfg.DNSPort == 0 {
		cfg.DNSPort = dns.DefaultPort
	}

	rt, err := daemon.New(cfg)
	if err != nil {
		return err
	}
	desktopText := "desactivada"
	if cfg.EnableDesktop {
		desktopText = fmt.Sprintf("127.0.0.1:%d", cfg.DesktopPort)
	}
	webView := rt.Engine.WebStatus()
	fmt.Fprintf(out, "lair daemon: api=%s dns=127.0.0.1:%d web=%s:%d/%d desktop=%s\n",
		rt.Socket(), cfg.DNSPort, webView.Bind, webView.HTTPPort, webView.HTTPSPort, desktopText)
	// OS signals must stop the daemon even when Run is called from a wrapper
	// that did not install its own handler.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	return rt.Run(ctx)
}

// parseFlagInt reads the value of a numeric flag and reports malformed or
// missing values instead of silently leaving a zero (which would change the
// daemon's ports).
func parseFlagInt(args []string, i *int, flag string, min, max int) (int, error) {
	if *i+1 >= len(args) {
		return 0, fmt.Errorf("%s requires a value", flag)
	}
	value, err := strconv.Atoi(args[*i+1])
	if err != nil || value < min || value > max {
		return 0, fmt.Errorf("invalid %s value %q (use %d-%d)", flag, args[*i+1], min, max)
	}
	*i++
	return value, nil
}

// mustPaths resolves the data paths without panicking on a broken environment.
func mustPaths() (app.Paths, error) {
	return app.Resolve()
}

func buildCatalog(paths app.Paths) (*manifest.Catalog, error) {
	pub, err := assets.PublicKey()
	if err != nil {
		return nil, err
	}
	base := defaultBaseURLs
	if v := os.Getenv("LAIR_MANIFEST_BASE_URLS"); v != "" {
		base = nil
		for _, item := range strings.Split(v, ",") {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				base = append(base, trimmed)
			}
		}
		if len(base) == 0 {
			return nil, fmt.Errorf("LAIR_MANIFEST_BASE_URLS contains no usable URLs")
		}
	}
	channel := strings.TrimSpace(os.Getenv("LAIR_CHANNEL"))
	if channel == "" {
		channel = "stable"
	}
	manifestName := "manifest.json"
	if channel != "stable" {
		manifestName = "manifest-" + channel + ".json"
	}
	catalog, err := manifest.NewCatalog(manifest.CatalogConfig{
		PublicKey: pub, Channel: channel, BaseURLs: base, Root: paths.Root,
		ManifestName: manifestName,
	})
	if err != nil {
		return nil, err
	}
	// Seed with the signed example catalog embedded in the binary so installs
	// work even when every mirror is unreachable. A newer cached or remote
	// manifest always takes precedence (Catalog.Load refuses older serials).
	if channel == "stable" {
		data, sig := assets.ExampleCatalog()
		if err := catalog.Load(data, sig); err != nil {
			return nil, fmt.Errorf("load embedded catalog: %w", err)
		}
	}
	return catalog, nil
}
