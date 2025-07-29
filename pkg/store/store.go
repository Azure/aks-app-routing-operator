package store

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/hashicorp/go-multierror"
)

// StoredFile represents a file entry in the store
type StoredFile struct {
	Path    string
	Content []byte
}

// Store manages local files with periodic refresh capabilities
type Store interface {
	AddFile(path string) error
	RemoveFile(path string)
	GetContent(path string) ([]byte, bool)
}

type store struct {
	mu            *sync.RWMutex
	files         map[string]*StoredFile
	refreshTicker *time.Ticker
	logger        logr.Logger
}

// New creates a new file store instance and starts periodic refresh
func New(logger logr.Logger, ctx context.Context, refreshInterval time.Duration) Store {
	s := &store{
		mu:     &sync.RWMutex{},
		files:  make(map[string]*StoredFile),
		logger: logger,
	}

	if refreshInterval > 0 {
		s.startPeriodicRefresh(ctx, refreshInterval)
	}

	return s
}

// AddFile adds a local file to the store for tracking
func (s *store) AddFile(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("file does not exist: %s", path)
	}

	// Read initial content
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", path, err)
	}

	s.files[path] = &StoredFile{
		Path:    path,
		Content: content,
	}

	s.logger.Info("Added file to store", "path", path, "size", len(content))
	return nil
}

// RemoveFile removes a file from the store
func (s *store) RemoveFile(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.files[path]; exists {
		delete(s.files, path)
		s.logger.Info("Removed file from store", "path", path)
	}
}

// GetContent returns just the content bytes for the given path
func (s *store) GetContent(path string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	file, exists := s.files[path]
	if !exists {
		return nil, false
	}

	// Return a copy to avoid external modifications
	return append([]byte(nil), file.Content...), true
}

// refreshFileInternal performs the actual refresh logic (must be called with lock held)
func (s *store) refreshFileInternal(path string, file *StoredFile) error {
	// Check if file still exists
	_, err := os.Stat(file.Path)
	if os.IsNotExist(err) {
		s.logger.Info("File no longer exists, removing from store", "path", path)
		delete(s.files, path)
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to stat file %s: %w", file.Path, err)
	}

	// Read updated content
	content, err := os.ReadFile(file.Path)
	if err != nil {
		return fmt.Errorf("failed to read updated file %s: %w", file.Path, err)
	}

	oldSize := len(file.Content)
	file.Content = content

	s.logger.Info("Refreshed file content",
		"path", path,
		"oldSize", oldSize,
		"newSize", len(content))

	return nil
}

// startPeriodicRefresh starts a goroutine that periodically refreshes all files
func (s *store) startPeriodicRefresh(ctx context.Context, interval time.Duration) {
	if s.refreshTicker != nil {
		s.logger.Info("Periodic refresh already running")
		return
	}

	s.refreshTicker = time.NewTicker(interval)
	s.logger.Info("Starting periodic file refresh", "interval", interval)

	go func() {
		for {
			select {
			case <-ctx.Done():
				s.logger.Info("Stopping periodic refresh due to context cancellation")
				return
			case <-s.refreshTicker.C:
				s.logger.Info("Performing periodic refresh")
				if err := s.refreshAll(); err != nil {
					s.logger.Error(err, "Error during periodic refresh")
				}
			}
		}
	}()
}

// refreshAll refreshes all files in the store
func (s *store) refreshAll() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result *multierror.Error
	for key, file := range s.files {
		if err := s.refreshFileInternal(key, file); err != nil {
			result = multierror.Append(result, fmt.Errorf("failed to refresh %s: %w", key, err))
		}
	}

	return result.ErrorOrNil()
}
