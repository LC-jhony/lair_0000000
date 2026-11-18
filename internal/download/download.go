// Package download implements verified artifact downloads and safe archive
// extraction. Nothing reaches the final destination until the SHA-256 matches
// and every archive member passes path-traversal checks.
package download

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	urlpkg "net/url"
	"os"
	"path/filepath"
	"strings"
)

// Progress is emitted while streaming a download.
type Progress struct {
	Phase      string // "downloading"
	Downloaded int64
	Total      int64
}

// Downloader fetches artifacts from a list of base URLs in order. Trust comes
// from the artifact's SHA-256, so any host may serve the bytes.
type Downloader struct {
	HTTP       *http.Client
	BaseURLs   []string
	OnProgress func(Progress)
}

func (d *Downloader) client() *http.Client {
	if d.HTTP != nil {
		return d.HTTP
	}
	return &http.Client{}
}

// DownloadTo streams the artifact to <destDir>/<name> via a .part file and a
// temporary directory on the same filesystem. On any failure it removes all
// traces and returns an error. The returned path is the verified file.
func (d *Downloader) DownloadTo(ctx context.Context, urlPath, wantSHA string, destDir, name string) (string, error) {
	return d.DownloadToProgress(ctx, urlPath, wantSHA, destDir, name, d.OnProgress)
}

// DownloadToProgress is DownloadTo with a per-call progress callback, so
// concurrent installs do not race over a shared callback field.
func (d *Downloader) DownloadToProgress(ctx context.Context, urlPath, wantSHA string, destDir, name string, onProgress func(Progress)) (string, error) {
	if wantSHA == "" || len(wantSHA) != 64 {
		return "", fmt.Errorf("artifact sha256 is required (64 hex chars)")
	}
	if name == "" || strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return "", fmt.Errorf("invalid destination name %q", name)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("create download dir: %w", err)
	}
	tmpDir, err := os.MkdirTemp(destDir, ".lair-dl-")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	part := filepath.Join(tmpDir, name+".part")
	var lastErr error
	for _, url := range resolveURLs(d.BaseURLs, urlPath) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		err := d.downloadOne(ctx, url, part, wantSHA, onProgress)
		if err == nil {
			// Hash matched; move to the verified name inside the temp dir.
			final := filepath.Join(tmpDir, name)
			if err := os.Rename(part, final); err != nil {
				return "", fmt.Errorf("rename verified file: %w", err)
			}
			// Move out of the temp dir to the caller's destination atomically.
			target := filepath.Join(destDir, name)
			if err := os.Rename(final, target); err != nil {
				return "", fmt.Errorf("move to destination: %w", err)
			}
			return target, nil
		}
		lastErr = err
		os.Remove(part)
	}
	return "", fmt.Errorf("download failed from all sources: %w", lastErr)
}

// resolveURLs keeps absolute artifact URLs as-is (the example catalog points
// directly at official vendor archives) and only prefixes relative ones with
// the configured mirror bases.
func resolveURLs(bases []string, urlPath string) []string {
	if strings.HasPrefix(urlPath, "http://") || strings.HasPrefix(urlPath, "https://") || strings.HasPrefix(urlPath, "file://") {
		return []string{urlPath}
	}
	out := make([]string, 0, len(bases))
	for _, base := range bases {
		out = append(out, strings.TrimSuffix(base, "/")+"/"+strings.TrimPrefix(urlPath, "/"))
	}
	return out
}

// openSource returns the body and its known size, for HTTP(S) and file:// URLs.
func (d *Downloader) openSource(ctx context.Context, url string) (io.ReadCloser, int64, error) {
	if strings.HasPrefix(url, "file://") {
		u, err := urlpkg.Parse(url)
		if err != nil {
			return nil, 0, err
		}
		f, err := os.Open(u.Path)
		if err != nil {
			return nil, 0, err
		}
		info, err := f.Stat()
		if err != nil {
			f.Close()
			return nil, 0, err
		}
		return f, info.Size(), nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := d.client().Do(req)
	if err != nil {
		return nil, 0, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, 0, fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}
	return resp.Body, resp.ContentLength, nil
}

func (d *Downloader) downloadOne(ctx context.Context, url, part, wantSHA string, onProgress func(Progress)) error {
	body, total, err := d.openSource(ctx, url)
	if err != nil {
		return err
	}
	defer body.Close()

	f, err := os.OpenFile(part, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	hash := sha256.New()
	buf := make([]byte, 64*1024)
	var written int64
	for {
		n, readErr := body.Read(buf)
		if n > 0 {
			if _, err := io.MultiWriter(f, hash).Write(buf[:n]); err != nil {
				return fmt.Errorf("write: %w", err)
			}
			written += int64(n)
			if onProgress != nil {
				onProgress(Progress{Phase: "downloading", Downloaded: written, Total: total})
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("read body: %w", readErr)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	got := hex.EncodeToString(hash.Sum(nil))
	if got != strings.ToLower(wantSHA) {
		return fmt.Errorf("sha256 mismatch: got %s, want %s", got, wantSHA)
	}
	return nil
}
