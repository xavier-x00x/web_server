package pool

import (
	"fmt"
	"os"
	"path/filepath"

	"gopherstack/internal/config"
)

// PHPConfigGenerator handles generating optimized php.ini files
type PHPConfigGenerator struct {
	cfg  *config.Config
	site config.SiteConfig
}

// NewPHPConfigGenerator creates a new PHP config generator
func NewPHPConfigGenerator(cfg *config.Config, site config.SiteConfig) *PHPConfigGenerator {
	return &PHPConfigGenerator{cfg: cfg, site: site}
}

// Generate creates an optimized php.ini file
func (g *PHPConfigGenerator) Generate(destPath string) error {
	opcacheStatus := "0"
	if g.cfg.EnableOpCache {
		opcacheStatus = "1"
	}

	// Resolve Log path relative to DocumentRoot (where workers run)
	logFile := fmt.Sprintf("php_errors_%s.log", g.site.Name)
	relLogPath, err := filepath.Rel(g.site.DocumentRoot, filepath.Join(g.cfg.LogDir, logFile))
	if err != nil {
		relLogPath = filepath.ToSlash(filepath.Join(g.cfg.LogDir, logFile)) // Fallback to absolute
	} else {
		relLogPath = filepath.ToSlash(relLogPath)
	}

	// Resolve absolute path for extensions
	phpDir := filepath.Dir(g.site.PHPBinaryPath)
	extDir := filepath.ToSlash(filepath.Join(phpDir, "ext"))

	content := fmt.Sprintf(`; GopherStack Enterprise - Generated PHP Configuration
; DO NOT EDIT MANUALLY - Changes will be overwritten by the Dashboard

[PHP]
extension_dir = "%s"
memory_limit = %dM


max_execution_time = 300
upload_max_filesize = 100M
post_max_size = 100M
display_errors = Off
log_errors = On
error_log = "%s"

[opcache]
zend_extension=opcache
opcache.enable=%s
opcache.memory_consumption=128
opcache.interned_strings_buffer=8
opcache.max_accelerated_files=4000
opcache.revalidate_freq=60
opcache.fast_shutdown=1

[ExtensionList]
; Common extensions usually enabled in Windows builds
extension=curl
extension=fileinfo
extension=gd
extension=mbstring
extension=openssl
extension=pdo_mysql
extension=sqlite3
extension=pdo_sqlite
`, extDir, g.cfg.MaxMemoryMB, relLogPath, opcacheStatus)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}

	return os.WriteFile(destPath, []byte(content), 0644)
}
