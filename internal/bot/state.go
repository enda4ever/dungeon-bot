package bot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type State struct {
	LogChannel    string               `json:"log_channel"`
	CrawlerRole   string               `json:"crawler_role"`
	DestroyerRole string               `json:"destroyer_role"`
	CrawlerSince  map[string]time.Time `json:"crawler_since"`
	PronounRoles  map[string]string    `json:"pronoun_roles"`
	Paused        bool                 `json:"paused"`
	Escapes       int                  `json:"escapes"`
	Deaths        int                  `json:"deaths"`
	Gold          int                  `json:"gold"`
	Items         map[string]int       `json:"items"`
}

type Store interface {
	Load() (*State, error)
	Save(*State) error
}

type FileStore struct {
	Path string
}

func (f FileStore) Load() (*State, error) {
	b, err := os.ReadFile(f.Path)
	if os.IsNotExist(err) {
		return &State{}, nil
	}
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (f FileStore) Save(s *State) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(f.Path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	tmp := f.Path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, f.Path)
}
