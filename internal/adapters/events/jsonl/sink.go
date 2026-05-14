package jsonl

import (
	"context"
	"encoding/json"
	"os"
	"sync"

	"github.com/diasYuri/agentflow/internal/core/run"
)

type Sink struct {
	mu   sync.Mutex
	file *os.File
	path string
}

func New(path string) (*Sink, error) {
	if path == "" {
		return &Sink{}, nil
	}
	return Open(path)
}

func Open(path string) (*Sink, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	return &Sink{file: file, path: path}, nil
}

func (s *Sink) Open(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file != nil {
		return nil
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	s.file = file
	s.path = path
	return nil
}

func (s *Sink) Emit(ctx context.Context, event run.Event) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return nil
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := s.file.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func (s *Sink) Close(ctx context.Context) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return nil
	}
	err := s.file.Close()
	s.file = nil
	return err
}
