package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
	"github.com/zalando/go-keyring"
)

type Profile struct {
	KeyID     string `json:"key_id,omitempty"`
	APIURL    string `json:"api_url"`
	Workspace string `json:"workspace"`
	Name      string `json:"name"`
	Scope     string `json:"scope"`
}
type Config struct {
	Active   string             `json:"active"`
	Profiles map[string]Profile `json:"profiles"`
}
type Project struct {
	Workspace string `toml:"workspace,omitempty"`
	Service   string `toml:"service,omitempty"`
	Project   string `toml:"project,omitempty"`
}
type Store struct{ Directory string }

func NewStore() (Store, error) {
	dir := Env("CONFIG_DIR")
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return Store{}, err
		}
		dir = DefaultDirectory(base)
	}
	return Store{dir}, nil
}
func (s Store) Load() (Config, error) {
	result := Config{Profiles: map[string]Profile{}}
	raw, err := os.ReadFile(filepath.Join(s.Directory, "config.json"))
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		return result, errors.New("invalid Openstead configuration; repair config.json before continuing")
	}
	if result.Profiles == nil {
		result.Profiles = map[string]Profile{}
	}
	return result, nil
}
func (s Store) Save(value Config) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return AtomicWrite(filepath.Join(s.Directory, "config.json"), append(raw, '\n'), 0600, false)
}
func CredentialID(profile Profile) string {
	sum := sha256.Sum256([]byte(profile.APIURL + "\n" + profile.Workspace + "\n" + profile.KeyID))
	return hex.EncodeToString(sum[:])
}

// The keychain service label is a storage contract shared with existing CLI/MCP installs.
func Token(profile Profile) (string, error) {
	token, err := keyring.Get("Runivo CLI", CredentialID(profile))
	if err != nil {
		return "", errors.New("no usable keychain credential; run openstead login, or set OPENSTEAD_API_KEY for CI")
	}
	return token, nil
}
func SetToken(profile Profile, token string) error {
	if err := keyring.Set("Runivo CLI", CredentialID(profile), token); err != nil {
		return errors.New("could not store the token in your OS keychain; use login --token-file PATH on a headless machine")
	}
	return nil
}
func DeleteToken(profile Profile) error {
	err := keyring.Delete("Runivo CLI", CredentialID(profile))
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
func LoadProject() (Project, error) {
	var p Project
	filename := "openstead.toml"
	raw, err := os.ReadFile(filename)
	if errors.Is(err, os.ErrNotExist) {
		filename = "runivo.toml"
		raw, err = os.ReadFile(filename)
	}
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	if len(raw) > 65536 {
		return p, fmt.Errorf("%s exceeds 64 KiB", filename)
	}
	if err = toml.Unmarshal(raw, &p); err != nil {
		return p, fmt.Errorf("invalid %s: %w", filename, err)
	}
	return p, nil
}
func AtomicWrite(path string, raw []byte, mode os.FileMode, exclusive bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if exclusive {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if err != nil {
			return err
		}
		_, err = file.Write(raw)
		closeErr := file.Close()
		if err != nil {
			return err
		}
		return closeErr
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".openstead-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if err = temp.Chmod(mode); err != nil {
		temp.Close()
		return err
	}
	if _, err = temp.Write(raw); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

// Env prefers the Openstead spelling while preserving existing CI environments.
// An explicitly empty Openstead value also overrides its legacy counterpart.
func Env(suffix string) string {
	if value, ok := os.LookupEnv("OPENSTEAD_" + suffix); ok {
		return value
	}
	return os.Getenv("RUNIVO_" + suffix)
}

// DefaultDirectory keeps existing profiles and their origin-bound keychain entries usable.
func DefaultDirectory(base string) string {
	current := filepath.Join(base, "openstead")
	legacy := filepath.Join(base, "runivo")
	if _, err := os.Stat(filepath.Join(current, "config.json")); !errors.Is(err, os.ErrNotExist) {
		return current
	}
	if _, err := os.Stat(filepath.Join(legacy, "config.json")); !errors.Is(err, os.ErrNotExist) {
		return legacy
	}
	return current
}
