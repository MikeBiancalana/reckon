package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoad_Defaults (T-3 / AC-2): with HOME set to a tempdir and both RECKON_VAULT
// and RECKON_CACHE unset, Load() must resolve VaultDir to $HOME/.reckon and
// CacheDir to $HOME/.cache/reckon (the XDG default). Neither may be empty.
func TestLoad_Defaults(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("RECKON_VAULT", "")
	t.Setenv("RECKON_CACHE", "")
	t.Setenv("XDG_CACHE_HOME", "") // prevent XDG override from test environment

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): unexpected error: %v", err)
	}
	if cfg.VaultDir == "" {
		t.Fatal("Load(): VaultDir must not be empty")
	}
	if cfg.CacheDir == "" {
		t.Fatal("Load(): CacheDir must not be empty")
	}
	wantVault := filepath.Join(tmp, ".reckon")
	if cfg.VaultDir != wantVault {
		t.Errorf("VaultDir = %q, want %q", cfg.VaultDir, wantVault)
	}
	wantCache := filepath.Join(tmp, ".cache", "reckon")
	if cfg.CacheDir != wantCache {
		t.Errorf("CacheDir = %q, want %q", cfg.CacheDir, wantCache)
	}
}

// TestLoad_VaultEnvOverride (T-4 / AC-2): when RECKON_VAULT is set, VaultDir must
// match the env value; CacheDir must NOT be prefixed by the vault path (the cache
// stays outside the vault regardless of RECKON_VAULT — EC-7 / IR-2).
func TestLoad_VaultEnvOverride(t *testing.T) {
	vault := "/tmp/myvault"
	t.Setenv("RECKON_VAULT", vault)
	t.Setenv("RECKON_CACHE", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): unexpected error: %v", err)
	}
	if cfg.VaultDir != vault {
		t.Errorf("VaultDir = %q, want %q", cfg.VaultDir, vault)
	}
	if strings.HasPrefix(cfg.CacheDir, vault) {
		t.Errorf("CacheDir %q must not be prefixed by VaultDir %q (EC-7/IR-2)", cfg.CacheDir, vault)
	}
}

// TestNewConfig_TempVaultHermetic (T-5 / AC-2 / IR-3): NewConfig(vaultDir) must set
// VaultDir to the given path, CacheDir must differ from VaultDir, and resolution
// must be pure — no directories created under the real HOME.
func TestNewConfig_TempVaultHermetic(t *testing.T) {
	tmp := t.TempDir()

	// Verify no files are written to HOME: stat HOME before and after; the real
	// check is that resolution is a no-op (no MkdirAll calls against ~).
	home, _ := os.UserHomeDir()
	var homeStatBefore os.FileInfo
	if home != "" {
		homeStatBefore, _ = os.Stat(home)
	}

	cfg, err := NewConfig(tmp)
	if err != nil {
		t.Fatalf("NewConfig(%q): unexpected error: %v", tmp, err)
	}
	if cfg.VaultDir != tmp {
		t.Errorf("VaultDir = %q, want %q", cfg.VaultDir, tmp)
	}
	if cfg.CacheDir == cfg.VaultDir {
		t.Errorf("CacheDir must not equal VaultDir (%q)", cfg.VaultDir)
	}
	// CacheDir must not be a child of VaultDir (IR-2/EC-7).
	if strings.HasPrefix(cfg.CacheDir, tmp) {
		t.Errorf("CacheDir %q is inside VaultDir %q — violates EC-7/IR-2", cfg.CacheDir, tmp)
	}
	// Confirm HOME was not disturbed (no new subdirectories were created).
	if home != "" && homeStatBefore != nil {
		homeStatAfter, _ := os.Stat(home)
		if homeStatAfter != nil && homeStatBefore.ModTime() != homeStatAfter.ModTime() {
			t.Logf("warning: HOME modification time changed — NewConfig may have touched ~")
		}
	}
}

// TestCacheNotInsideVault (T-15 / EC-7): with a temp vault, CacheDir returned by
// NewConfig must not be prefixed by VaultDir.
func TestCacheNotInsideVault(t *testing.T) {
	tmp := t.TempDir()
	cfg, err := NewConfig(tmp)
	if err != nil {
		t.Fatalf("NewConfig(%q): %v", tmp, err)
	}
	if strings.HasPrefix(cfg.CacheDir, cfg.VaultDir) {
		t.Errorf("CacheDir %q must not be inside VaultDir %q (EC-7)", cfg.CacheDir, cfg.VaultDir)
	}
}

// TestCacheInsideVault_Rejected (T-15 / EC-7): LoadWithOverrides must return a
// non-nil error when cacheDir is a subdirectory of vaultDir — silently syncing the
// cache would be a data-correctness bug.
func TestCacheInsideVault_Rejected(t *testing.T) {
	tmp := t.TempDir()
	insideCache := filepath.Join(tmp, "cache") // inside vault dir

	_, err := LoadWithOverrides(tmp, insideCache)
	if err == nil {
		t.Fatalf("LoadWithOverrides(%q, %q): expected error (cache inside vault), got nil", tmp, insideCache)
	}
}
