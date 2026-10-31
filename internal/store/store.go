// Package store 用单个 JSON 文件保存全部状态，写入走临时文件 + rename，
// 避免断电或并发写导致文件被截断。数据量只有几十 KB，不需要数据库。
package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"towerdocker/internal/model"
)

type Store struct {
	mu    sync.RWMutex
	path  string
	state *model.State
}

func Open(path string) (*Store, error) {
	s := &Store{path: path}
	b, readErr := os.ReadFile(path)
	if readErr == nil {
		var st model.State
		if err := json.Unmarshal(b, &st); err != nil {
			return nil, err
		}
		s.state = &st
		s.state.Normalize()
		return s, nil
	}
	if !os.IsNotExist(readErr) {
		return nil, readErr
	}
	s.state = &model.State{}
	s.state.Normalize()
	return s, s.flush()
}

func clone(st *model.State) *model.State {
	b, err := json.Marshal(st)
	if err != nil {
		return st
	}
	var out model.State
	if err := json.Unmarshal(b, &out); err != nil {
		return st
	}
	return &out
}

// Get 返回状态副本，调用方随便改都不会影响已落盘的数据。
func (s *Store) Get() *model.State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return clone(s.state)
}

// Update 在副本上执行 fn，成功才提交并落盘。
func (s *Store) Update(fn func(*model.State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := clone(s.state)
	if err := fn(next); err != nil {
		return err
	}
	next.Normalize()
	s.state = next
	return s.flush()
}

func (s *Store) flush() error {
	b, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	return nil
}

func Now() string {
	return time.Now().Format("2006-01-02 15:04:05")
}
