package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestConfigRoundtripAndPrivatePermissions(t *testing.T) {
	store := Store{t.TempDir()}
	value := Config{Active: "test", Profiles: map[string]Profile{"test": {APIURL: "https://example.com", Workspace: "w", Name: "name", KeyID: "key"}}}
	if e := store.Save(value); e != nil {
		t.Fatal(e)
	}
	got, e := store.Load()
	if e != nil || got.Active != "test" || got.Profiles["test"].Name != "name" {
		t.Fatal(got, e)
	}
	value.Active = "next"
	if e = store.Save(value); e != nil {
		t.Fatal("atomic replacement:", e)
	}
	info, _ := os.Stat(filepath.Join(store.Directory, "config.json"))
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatal("configuration permissions")
	}
}
func TestCorruptConfigIsNotSilentlyReplaced(t *testing.T) {
	store := Store{t.TempDir()}
	_ = os.WriteFile(filepath.Join(store.Directory, "config.json"), []byte("broken"), 0600)
	if _, e := store.Load(); e == nil {
		t.Fatal("accepted broken config")
	}
}
func TestExclusiveCredentialWriteAndKeyIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	if e := AtomicWrite(path, []byte("first"), 0600, true); e != nil {
		t.Fatal(e)
	}
	if e := AtomicWrite(path, []byte("second"), 0600, true); e == nil {
		t.Fatal("overwrote credential")
	}
	a := Profile{APIURL: "https://a.test", Workspace: "same", KeyID: "first"}
	b := a
	b.KeyID = "second"
	if CredentialID(a) == CredentialID(b) {
		t.Fatal("profiles share credential slot")
	}
}

func TestOpensteadEnvironmentAndExistingProfileDirectory(t *testing.T) {
	t.Setenv("RUNIVO_API_KEY", "legacy")
	if Env("API_KEY") != "legacy" {
		t.Fatal("legacy environment was ignored")
	}
	t.Setenv("OPENSTEAD_API_KEY", "current")
	if Env("API_KEY") != "current" {
		t.Fatal("new environment did not take precedence")
	}
	t.Setenv("OPENSTEAD_API_KEY", "")
	if Env("API_KEY") != "" {
		t.Fatal("explicit empty environment unexpectedly reused legacy credentials")
	}
	base := t.TempDir()
	current, legacy := filepath.Join(base, "openstead"), filepath.Join(base, "runivo")
	if DefaultDirectory(base) != current {
		t.Fatal("new installations use legacy directory")
	}
	if err := AtomicWrite(filepath.Join(legacy, "config.json"), []byte(`{}`), 0600, false); err != nil {
		t.Fatal(err)
	}
	if DefaultDirectory(base) != legacy {
		t.Fatal("existing profiles were orphaned")
	}
	if err := AtomicWrite(filepath.Join(current, "config.json"), []byte(`{}`), 0600, false); err != nil {
		t.Fatal(err)
	}
	if DefaultDirectory(base) != current {
		t.Fatal("new profiles do not take precedence")
	}
}

func TestProjectFilenameFallbackAndMalformedNewFile(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("runivo.toml", []byte(`service = "legacy"`), 0600); err != nil {
		t.Fatal(err)
	}
	if p, err := LoadProject(); err != nil || p.Service != "legacy" {
		t.Fatal(p, err)
	}
	if err := os.WriteFile("openstead.toml", []byte(`service = "current"`), 0600); err != nil {
		t.Fatal(err)
	}
	if p, err := LoadProject(); err != nil || p.Service != "current" {
		t.Fatal(p, err)
	}
	if err := os.WriteFile("openstead.toml", []byte(`invalid = [`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadProject(); err == nil {
		t.Fatal("malformed current config silently fell back to legacy")
	}
}
