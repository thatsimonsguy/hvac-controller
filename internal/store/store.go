package store

import (
	"encoding/json"
	"os"

	"github.com/thatsimonsguy/hvac-controller/internal/model"
)

type Store struct {
	path string
}

func New(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Load() (*model.SystemState, error) {
	file, err := os.Open(s.path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var state model.SystemState
	if err := json.NewDecoder(file).Decode(&state); err != nil {
		return nil, err
	}
	return &state, nil
}

func (s *Store) Save(state *model.SystemState) error {
	tmpPath := s.path + ".tmp"

	file, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(state); err != nil {
		file.Close()
		return err
	}
	file.Sync()
	file.Close()

	return os.Rename(tmpPath, s.path)
}
