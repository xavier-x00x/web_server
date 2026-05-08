package pool

import (
	"fmt"
	"os"
	"path/filepath"

	"gopherstack/internal/config"
)

// PHPConfigGenerator handles generating optimized php.ini files
type PHPConfigGenerator struct {
	cfg *config.Config
}

// NewPHPConfigGenerator creates a new PHP config generator
func NewPHPConfigGenerator(cfg *config.Config) *PHPConfigGenerator {
	return &PHPConfigGenerator{cfg: cfg}
}

// Generate creates an optimized php.ini file
func (g *PHPConfigGenerator) Generate(destPath string) error {
	opcacheStatus := "0"
	if g.cfg.EnableOpCache {
		opcacheStatus = "1"
	}

	// Resolve Log path relative to DocumentRoot (where workers run)
	relLogPath, err := filepath.Rel(g.cfg.DocumentRoot, filepath.Join(g.cfg.LogDir, "php_errors.log"))
	if err != nil {
		relLogPath = filepath.ToSlash(filepath.Join(g.cfg.LogDir, "php_errors.log")) // Fallback to absolute
	} else {
		relLogPath = filepath.ToSlash(relLogPath)
	}

	content := fmt.Sprintf(`; GopherStack Enterprise - Generated PHP Configuration
; DO NOT EDIT MANUALLY - Changes will be overwritten by the Dashboard

[PHP]
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
`, g.cfg.MaxMemoryMB, relLogPath, opcacheStatus)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}

	return os.WriteFile(destPath, []byte(content), 0644)
}
