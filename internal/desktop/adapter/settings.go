package adapter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// SettingsStore define a interface para persistencia de AppSettings.
type SettingsStore interface {
	Load() (AppSettings, error)
	Save(settings AppSettings) error
}

// JSONSettingsStore persiste AppSettings em arquivo JSON.
type JSONSettingsStore struct {
	path string
	mu   sync.RWMutex
}

// NewJSONSettingsStore cria um store com o caminho informado.
func NewJSONSettingsStore(path string) *JSONSettingsStore {
	return &JSONSettingsStore{path: path}
}

// Load le as configuracoes do disco ou retorna defaults.
func (s *JSONSettingsStore) Load() (AppSettings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultSettings(), nil
		}
		return AppSettings{}, err
	}

	var settings AppSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return AppSettings{}, err
	}
	return settings, nil
}

// Save grava as configuracoes no disco.
func (s *JSONSettingsStore) Save(settings AppSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.path, data, 0o644)
}

func defaultSettings() AppSettings {
	return AppSettings{
		Theme:     "system",
		LogFormat: "text",
	}
}

// DefaultSettingsPath retorna o caminho padrao para o arquivo de configuracoes.
func DefaultSettingsPath() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".agentflow", "desktop-settings.json")
	}
	return filepath.Join(".agentflow", "desktop-settings.json")
}
