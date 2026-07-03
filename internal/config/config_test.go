package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoad_Defaults (T-3 / AC-2): with HOME set to a tempdir and both RECKON_VAULT
// and RECKON_CACHE unset, Load() must resolve VaultDir to $HOME/reckon and
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
	wantVault := filepath.Join(tmp, "reckon")
	if cfg.VaultDir != wantVault {
		t.Errorf("VaultDir = %q, want %q", cfg.VaultDir, wantVault)
	}
	wantCache := filepath.Join(tmp, ".cache", "reckon")
	if cfg.CacheDir != wantCache {
		t.Errorf("CacheDir = %q, want %q", cfg.CacheDir, wantCache)
	}
}

// TestLoad_DefaultVault_NotLegacyDir (reckon-lrw2, AC-1(a)/(b)): with no env
// overrides, Load()'s default VaultDir must no longer resolve to the legacy
// $HOME/.reckon path — it must resolve to the new $HOME/reckon default instead.
func TestLoad_DefaultVault_NotLegacyDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("RECKON_VAULT", "")
	t.Setenv("RECKON_CACHE", "")
	t.Setenv("RECKON_DATA_DIR", "")
	t.Setenv("XDG_CACHE_HOME", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): unexpected error: %v", err)
	}

	legacy := filepath.Join(tmp, ".reckon")
	if cfg.VaultDir == legacy {
		t.Errorf("VaultDir = %q, must NOT equal legacy dir %q", cfg.VaultDir, legacy)
	}

	want := filepath.Join(tmp, "reckon")
	if cfg.VaultDir != want {
		t.Errorf("VaultDir = %q, want %q", cfg.VaultDir, want)
	}
}

// TestLoad_VaultEnvOverride (T-4 / AC-2): when RECKON_VAULT is set, VaultDir must
// match the env value; CacheDir must NOT be prefixed by the vault path (the cache
// stays outside the vault regardless of RECKON_VAULT — EC-7 / IR-2).
func TestLoad_VaultEnvOverride(t *testing.T) {
	vault := "/tmp/myvault"
	t.Setenv("RECKON_VAULT", vault)
	t.Setenv("RECKON_CACHE", "")
	// Clear XDG_CACHE_HOME so an ambient value (possibly inside the vault) cannot
	// trip the cache-inside-vault guard and fail this test for the wrong reason.
	t.Setenv("XDG_CACHE_HOME", "")

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
			t.Errorf("HOME modification time changed — NewConfig touched ~ (must be pure, AC-2/IR-3)")
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

// relInside reports whether child equals, or is nested inside, parent. Mirrors the
// filepath.Rel-based technique used by the EC-7 cache-inside-vault guard in
// config.go (LoadWithOverrides).
func relInside(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel == "." || !strings.HasPrefix(rel, "..")
}

// TestDefaults_LegacyVaultNonCollision (reckon-lrw2, AC-1(b)): the legacy DataDir()
// default and the new Config.VaultDir default must resolve to different,
// non-nesting paths for the same $HOME. The legacy path is computed as a literal
// here (not via DataDir()) so this test stays free of DataDir()'s mkdir side
// effect.
func TestDefaults_LegacyVaultNonCollision(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("RECKON_VAULT", "")
	t.Setenv("RECKON_CACHE", "")
	t.Setenv("RECKON_DATA_DIR", "")
	t.Setenv("XDG_CACHE_HOME", "")

	legacy := filepath.Join(tmp, ".reckon") // literal legacy default, avoids DataDir() mkdir

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): unexpected error: %v", err)
	}

	if cfg.VaultDir == legacy {
		t.Fatalf("new vault default %q must not equal legacy default %q", cfg.VaultDir, legacy)
	}
	if relInside(legacy, cfg.VaultDir) {
		t.Errorf("legacy dir %q must not be nested inside new vault dir %q", legacy, cfg.VaultDir)
	}
	if relInside(cfg.VaultDir, legacy) {
		t.Errorf("new vault dir %q must not be nested inside legacy dir %q", cfg.VaultDir, legacy)
	}
}

// TestDataDir_LegacyDefault_Unchanged (reckon-lrw2, implicit requirement "no
// breakage of shipped legacy callers"): DataDir() and all its derivatives must
// keep resolving under the legacy $HOME/.reckon default, unaffected by the vault
// default relocation.
func TestDataDir_LegacyDefault_Unchanged(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("RECKON_DATA_DIR", "")
	t.Setenv("RECKON_VAULT", "")
	t.Setenv("RECKON_CACHE", "")
	t.Setenv("XDG_CACHE_HOME", "")

	dataDir, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir(): unexpected error: %v", err)
	}
	want := filepath.Join(tmp, ".reckon")
	if dataDir != want {
		t.Fatalf("DataDir() = %q, want %q", dataDir, want)
	}

	cases := []struct {
		name string
		fn   func() (string, error)
		want string
	}{
		{"JournalDir", JournalDir, filepath.Join(want, "journal")},
		{"TasksDir", TasksDir, filepath.Join(want, "tasks")},
		{"NotesDir", NotesDir, filepath.Join(want, "notes")},
		{"DatabasePath", DatabasePath, filepath.Join(want, "reckon.db")},
		{"LogDir", LogDir, filepath.Join(want, "logs")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := c.fn()
			if err != nil {
				t.Fatalf("%s(): unexpected error: %v", c.name, err)
			}
			if got != c.want {
				t.Errorf("%s() = %q, want %q", c.name, got, c.want)
			}
		})
	}
}

// TestLoad_DefaultVault_Pure (reckon-lrw2, AC-1(c)): resolving Load()'s default
// VaultDir must not create the directory (or anything else) on disk, even against
// a fresh $HOME with no pre-existing subdirectories.
func TestLoad_DefaultVault_Pure(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("RECKON_VAULT", "")
	t.Setenv("RECKON_CACHE", "")
	t.Setenv("RECKON_DATA_DIR", "")
	t.Setenv("XDG_CACHE_HOME", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): unexpected error: %v", err)
	}
	if cfg.VaultDir == "" {
		t.Fatal("Load(): VaultDir must not be empty")
	}
	if _, statErr := os.Stat(cfg.VaultDir); !os.IsNotExist(statErr) {
		t.Errorf("Load(): VaultDir %q must not exist on disk after pure resolution (stat err = %v)", cfg.VaultDir, statErr)
	}
}

// TestLoad_VaultEnvOverride_LegacyPath (reckon-lrw2, EC "RECKON_VAULT explicitly
// set to the legacy path"): explicitly pointing RECKON_VAULT at the legacy
// $HOME/.reckon path must still be honored — the default collision fix must not
// block the escape hatch.
func TestLoad_VaultEnvOverride_LegacyPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	legacy := filepath.Join(tmp, ".reckon")
	t.Setenv("RECKON_VAULT", legacy)
	t.Setenv("RECKON_CACHE", "")
	t.Setenv("RECKON_DATA_DIR", "")
	t.Setenv("XDG_CACHE_HOME", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): unexpected error: %v", err)
	}
	if cfg.VaultDir != legacy {
		t.Errorf("VaultDir = %q, want %q (explicit RECKON_VAULT override honored)", cfg.VaultDir, legacy)
	}
}

// TestEnvIndependence_DataDirVsVault (reckon-lrw2, implicit requirement
// "RECKON_DATA_DIR and RECKON_VAULT remain distinct"): overriding RECKON_DATA_DIR
// must not affect Config.VaultDir's default resolution.
func TestEnvIndependence_DataDirVsVault(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	override := filepath.Join(t.TempDir(), "legacy-override")
	t.Setenv("RECKON_DATA_DIR", override)
	t.Setenv("RECKON_VAULT", "")
	t.Setenv("RECKON_CACHE", "")
	t.Setenv("XDG_CACHE_HOME", "")

	dataDir, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir(): unexpected error: %v", err)
	}
	if dataDir != override {
		t.Errorf("DataDir() = %q, want %q (RECKON_DATA_DIR override)", dataDir, override)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): unexpected error: %v", err)
	}
	want := filepath.Join(tmp, "reckon")
	if cfg.VaultDir != want {
		t.Errorf("VaultDir = %q, want %q (new default, unaffected by RECKON_DATA_DIR)", cfg.VaultDir, want)
	}
}
