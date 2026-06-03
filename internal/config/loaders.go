package config

import (
	"fmt"
	"strings"

	"github.com/BurntSushi/toml"
)

type ConfigWarning struct {
	Path    string
	Key     string
	Message string
}

func (w ConfigWarning) String() string {
	return fmt.Sprintf("%s: %s", w.Path, w.Message)
}

func LoadGlobalConfig(path string) (*GlobalConfig, error) {
	cfg, _, err := LoadGlobalConfigWithWarnings(path)
	return cfg, err
}

func LoadGlobalConfigWithWarnings(path string) (*GlobalConfig, []ConfigWarning, error) {
	cfg := GetDefaultGlobalConfig()
	defaults := GetDefaultGlobalConfig()
	meta, err := toml.DecodeFile(path, cfg)
	if err != nil {
		return cfg, nil, err
	}

	warnings := collectGlobalConfigWarnings(path, meta, cfg, defaults)
	return cfg, warnings, nil
}

func LoadYTConfig(path string) (*YTConfig, error) {
	cfg, _, err := LoadYTConfigWithWarnings(path)
	return cfg, err
}

func LoadYTConfigWithWarnings(path string) (*YTConfig, []ConfigWarning, error) {
	cfg := GetDefaultYTConfig()
	defaults := GetDefaultYTConfig()
	meta, err := toml.DecodeFile(path, cfg)
	if err != nil {
		return cfg, nil, err
	}

	warnings := collectYTConfigWarnings(path, meta, cfg, defaults)
	return cfg, warnings, nil
}

func LoadTwitchConfig(path string) (*TwitchConfig, error) {
	cfg, _, err := LoadTwitchConfigWithWarnings(path)
	return cfg, err
}

func LoadTwitchConfigWithWarnings(path string) (*TwitchConfig, []ConfigWarning, error) {
	cfg := GetDefaultTwitchConfig()
	defaults := GetDefaultTwitchConfig()
	meta, err := toml.DecodeFile(path, cfg)
	if err != nil {
		return cfg, nil, err
	}

	warnings := collectTwitchConfigWarnings(path, meta, cfg, defaults)
	return cfg, warnings, nil
}

func addMissingWarning(warnings *[]ConfigWarning, path string, meta toml.MetaData, key []string, defaultValue any) {
	if meta.IsDefined(key...) {
		return
	}

	keyName := strings.Join(key, ".")
	*warnings = append(*warnings, ConfigWarning{
		Path:    path,
		Key:     keyName,
		Message: fmt.Sprintf("missing %s; using default %s", keyName, formatDefault(defaultValue)),
	})
}

func addInvalidWarning(warnings *[]ConfigWarning, path, key string, value, defaultValue any, reason string) {
	*warnings = append(*warnings, ConfigWarning{
		Path:    path,
		Key:     key,
		Message: fmt.Sprintf("invalid %s=%s (%s); using default %s", key, formatDefault(value), reason, formatDefault(defaultValue)),
	})
}

func addDeprecatedWarning(warnings *[]ConfigWarning, path, oldKey, newKey string) {
	*warnings = append(*warnings, ConfigWarning{
		Path:    path,
		Key:     oldKey,
		Message: fmt.Sprintf("deprecated %s; use %s instead", oldKey, newKey),
	})
}

func addUndecodedWarnings(warnings *[]ConfigWarning, path string, meta toml.MetaData) {
	for _, key := range meta.Undecoded() {
		keyName := key.String()
		*warnings = append(*warnings, ConfigWarning{
			Path:    path,
			Key:     keyName,
			Message: fmt.Sprintf("unknown %s; value is ignored", keyName),
		})
	}
}

func formatDefault(value any) string {
	switch v := value.(type) {
	case string:
		return fmt.Sprintf("%q", v)
	case []string:
		quoted := make([]string, 0, len(v))
		for _, item := range v {
			quoted = append(quoted, fmt.Sprintf("%q", item))
		}
		return "[" + strings.Join(quoted, ", ") + "]"
	default:
		return fmt.Sprintf("%v", v)
	}
}
