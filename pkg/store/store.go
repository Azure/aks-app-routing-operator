package store

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/go-logr/logr"
)

// storedFile represents a file entry in the store
type storedFile struct {
	Path    string
	Content []byte
}

// RotationEvent represents a file rotation/change event
type RotationEvent struct {
	Path string
}

// Store manages local files with filesystem watching capabilities
type Store interface {
	AddFile(path string) error
	GetContent(path string) ([]byte, bool)
	RotationEvents() <-chan RotationEvent
	Errors() <-chan error
}

type store struct {
	mu         *sync.RWMutex
	files      map[string]*storedFile
	watcher    *fsnotify.Watcher
	ctx        context.Context
	logger     logr.Logger
	rotationCh chan RotationEvent
	errorCh    chan error
}

// New creates a new file store instance with filesystem watching
func New(logger logr.Logger, ctx context.Context) (Store, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create filesystem watcher: %w", err)
	}

	s := &store{
		mu:         &sync.RWMutex{},
		files:      make(map[string]*storedFile),
		watcher:    watcher,
		ctx:        ctx,
		logger:     logger,
		rotationCh: make(chan RotationEvent, 100), // Buffered to prevent blocking
		errorCh:    make(chan error, 100),         // Buffered to prevent blocking
	}

	// Start watching for filesystem events
	s.startWatching()

	return s, nil
}

// AddFile adds a local file to the store for tracking
func (s *store) AddFile(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := os.Stat(path)
	// Check if file exists
	if os.IsNotExist(err) {
		return fmt.Errorf("file does not exist: %s", path)
	}
	if err != nil {
		return fmt.Errorf("failed to stat file %s: %w", path, err)
	}

	// Check if file is already being watched
	if _, exists := s.files[path]; exists {
		return fmt.Errorf("file already exists in store: %s", path)
	}

	// Read initial content
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", path, err)
	}

	// Add to watcher
	if err := s.watcher.Add(path); err != nil {
		return fmt.Errorf("failed to add file to watcher %s: %w", path, err)
	}

	s.files[path] = &storedFile{
		Path:    path,
		Content: content,
	}

	s.logger.Info("Added file to store", "path", path, "size", len(content))

	// Note: No rotation event sent for "added" - only for "updated" and "removed"

	return nil
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

// RotationEvents returns a read-only channel for rotation events
func (s *store) RotationEvents() <-chan RotationEvent {
	return s.rotationCh
}

// Errors returns a read-only channel for errors
func (s *store) Errors() <-chan error {
	return s.errorCh
}

// startWatching starts a goroutine that handles filesystem events
func (s *store) startWatching() {
	go func() {
		s.logger.Info("Starting filesystem watcher")

		// Ensure cleanup when goroutine exits
		defer func() {
			s.logger.Info("Cleaning up filesystem watcher resources")
			close(s.rotationCh)
			close(s.errorCh)
			s.watcher.Close()
		}()

		for {
			select {
			case <-s.ctx.Done():
				s.logger.Info("Stopping filesystem watcher due to context cancellation")
				return
			case event, ok := <-s.watcher.Events:
				if !ok {
					s.logger.Info("Filesystem watcher events channel closed")
					return
				}
				s.handleFileEvent(event)
			case err, ok := <-s.watcher.Errors:
				if !ok {
					s.logger.Info("Filesystem watcher errors channel closed")
					return
				}
				s.logger.Error(err, "Filesystem watcher error")

				// Send error to error channel
				select {
				case s.errorCh <- err:
				default:
					s.logger.Info("Error channel full, dropping error")
				}
			}
		}
	}()
}

// handleFileEvent processes a filesystem event
func (s *store) handleFileEvent(event fsnotify.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := event.Name

	// Check if this file is being tracked
	file, exists := s.files[path]
	if !exists {
		return
	}

	s.logger.Info("File event received", "path", path, "op", event.Op.String())

	// Handle different event types
	if event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) {
		// File was deleted or renamed - return error
		err := fmt.Errorf("file was deleted or renamed: %s", path)
		s.logger.Error(err, "File no longer exists")

		// Send error to error channel
		select {
		case s.errorCh <- err:
		default:
			s.logger.Info("Error channel full, dropping error")
		}
		return
	}

	if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
		// File was modified or created, reload content
		if err := s.refreshFileContent(path, file); err != nil {
			s.logger.Error(err, "Failed to refresh file content", "path", path)

			// Send error to error channel
			select {
			case s.errorCh <- err:
			default:
				s.logger.Info("Error channel full, dropping error")
			}
		} else {
			// Send rotation event for file update
			select {
			case s.rotationCh <- RotationEvent{Path: path}:
			default:
				s.logger.Info("Rotation channel full, dropping event", "path", path)
			}
		}
	}
}

// refreshFileContent reloads the content of a specific file (must be called with lock held)
func (s *store) refreshFileContent(path string, file *storedFile) error {
	// Check if file still exists
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("file no longer exists: %s", path)
	}
	if err != nil {
		return fmt.Errorf("failed to stat file %s: %w", path, err)
	}

	// Read updated content
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read updated file %s: %w", path, err)
	}

	oldSize := len(file.Content)
	file.Content = content

	s.logger.Info("Refreshed file content",
		"path", path,
		"oldSize", oldSize,
		"newSize", len(content))

	return nil
}
