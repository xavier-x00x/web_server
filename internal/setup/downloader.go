package setup

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gopherstack/internal/config"
)

const (
	phpDownloadURL   = "https://downloads.php.net/~windows/releases/archives/php-8.5.6-nts-Win32-vs17-x64.zip"
	nginxDownloadURL = "https://nginx.org/download/nginx-1.30.0.zip"
	phpZipName       = "php-8.5.6-nts-Win32-vs17-x64.zip"
	nginxZipName     = "nginx-1.30.0.zip"
)

// ProgressCallback is called during download with current and total bytes
type ProgressCallback func(downloaded, total int64, name string)

// Downloader handles downloading and extracting PHP and Nginx binaries
type Downloader struct {
	cfg      *config.Config
	progress ProgressCallback
}

// NewDownloader creates a new Downloader
func NewDownloader(cfg *config.Config, progress ProgressCallback) *Downloader {
	if progress == nil {
		progress = func(downloaded, total int64, name string) {}
	}
	return &Downloader{cfg: cfg, progress: progress}
}

// EnsureBinaries checks and downloads PHP and Nginx if not present
func (d *Downloader) EnsureBinaries() error {
	phpNeeded := !fileExists(d.cfg.PHPBinaryPath)
	nginxNeeded := !fileExists(d.cfg.NginxBinaryPath)

	if !phpNeeded && !nginxNeeded {
		fmt.Println("[setup] All binaries already present, skipping download.")
		return nil
	}

	if phpNeeded {
		fmt.Println("[setup] PHP binary not found, downloading...")
		if err := d.downloadAndExtractPHP(); err != nil {
			return fmt.Errorf("failed to download PHP: %w", err)
		}
		fmt.Println("[setup] PHP downloaded and extracted successfully.")
	}

	if nginxNeeded {
		fmt.Println("[setup] Nginx binary not found, downloading...")
		if err := d.downloadAndExtractNginx(); err != nil {
			return fmt.Errorf("failed to download Nginx: %w", err)
		}
		fmt.Println("[setup] Nginx downloaded and extracted successfully.")
	}

	return nil
}

func (d *Downloader) downloadAndExtractPHP() error {
	phpDir := filepath.Dir(d.cfg.PHPBinaryPath)
	if err := os.MkdirAll(phpDir, 0755); err != nil {
		return err
	}

	tmpZip := filepath.Join(d.cfg.BinDir, phpZipName)
	defer os.Remove(tmpZip)

	if err := d.downloadFile(phpDownloadURL, tmpZip, "PHP 8.5.6"); err != nil {
		return err
	}

	// PHP zip extracts files directly (no subdirectory)
	return d.extractZip(tmpZip, phpDir, "")
}

func (d *Downloader) downloadAndExtractNginx() error {
	nginxDir := filepath.Dir(d.cfg.NginxBinaryPath)
	if err := os.MkdirAll(nginxDir, 0755); err != nil {
		return err
	}

	tmpZip := filepath.Join(d.cfg.BinDir, nginxZipName)
	defer os.Remove(tmpZip)

	if err := d.downloadFile(nginxDownloadURL, tmpZip, "Nginx 1.30.0"); err != nil {
		return err
	}

	// Nginx zip has a subdirectory like "nginx-1.30.0/"
	// We need to strip this prefix and extract to bin/nginx/
	return d.extractZip(tmpZip, nginxDir, "nginx-1.30.0")
}

func (d *Downloader) downloadFile(url, destPath, name string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	total := resp.ContentLength
	var downloaded int64

	buf := make([]byte, 32*1024) // 32KB buffer
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			_, writeErr := out.Write(buf[:n])
			if writeErr != nil {
				return fmt.Errorf("failed to write file: %w", writeErr)
			}
			downloaded += int64(n)
			d.progress(downloaded, total, name)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("failed to read response: %w", readErr)
		}
	}

	fmt.Printf("\n[setup] Downloaded %s (%d MB)\n", name, downloaded/(1024*1024))
	return nil
}

// extractZip extracts a zip file to destDir, optionally stripping a prefix directory
func (d *Downloader) extractZip(zipPath, destDir, stripPrefix string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("failed to open zip: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		name := f.Name

		// Strip prefix directory if specified
		if stripPrefix != "" {
			if !strings.HasPrefix(name, stripPrefix+"/") && name != stripPrefix+"/" {
				continue
			}
			name = strings.TrimPrefix(name, stripPrefix+"/")
			if name == "" {
				continue
			}
		}

		// Security: prevent path traversal
		destPath := filepath.Join(destDir, filepath.FromSlash(name))
		if !strings.HasPrefix(destPath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			// Allow exact match of destDir itself
			if destPath != filepath.Clean(destDir) {
				continue
			}
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(destPath, 0755)
			continue
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}

		outFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
