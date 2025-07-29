package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_AddFile(t *testing.T) {
	// Create a temporary directory for test files
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	testContent := "Hello, World!"

	// Create test file
	err := os.WriteFile(testFile, []byte(testContent), 0o644)
	require.NoError(t, err)

	// Create store with fsnotify
	ctx := context.Background()
	store, err := New(logr.Discard(), ctx)
	require.NoError(t, err)

	// Test adding file
	err = store.AddFile(testFile)
	require.NoError(t, err)

	// Verify file was added
	content, exists := store.GetContent(testFile)
	assert.True(t, exists)
	assert.Equal(t, testContent, string(content))

	// Test adding non-existent file
	err = store.AddFile("/path/that/does/not/exist")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "file does not exist")

	// Test adding the same file again
	err = store.AddFile(testFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestStore_GetContent(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	testContent := "test content"

	err := os.WriteFile(testFile, []byte(testContent), 0o644)
	require.NoError(t, err)

	ctx := context.Background()
	store, err := New(logr.Discard(), ctx)
	require.NoError(t, err)

	err = store.AddFile(testFile)
	require.NoError(t, err)

	// Test GetContent
	content, exists := store.GetContent(testFile)
	require.True(t, exists)
	assert.Equal(t, testContent, string(content))

	// Test non-existent file
	_, exists = store.GetContent("/nonexistent/path")
	assert.False(t, exists)
}

func TestStore_FileWatcher(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	originalContent := "original"
	updatedContent := "updated"

	// Create test file
	err := os.WriteFile(testFile, []byte(originalContent), 0o644)
	require.NoError(t, err)

	// Create store with fsnotify
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store, err := New(logr.Discard(), ctx)
	require.NoError(t, err)

	err = store.AddFile(testFile)
	require.NoError(t, err)

	// Ensure file is in store
	content, exists := store.GetContent(testFile)
	require.True(t, exists)
	assert.Equal(t, originalContent, string(content))

	// Wait a bit, then update file
	time.Sleep(10 * time.Millisecond)
	err = os.WriteFile(testFile, []byte(updatedContent), 0o644)
	require.NoError(t, err)

	// Wait for fsnotify to pick up the change
	assert.Eventually(t, func() bool {
		content, exists := store.GetContent(testFile)
		return exists && string(content) == updatedContent
	}, 200*time.Millisecond, 10*time.Millisecond)
}

func TestStore_ConcurrentAccess(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")

	// Create test file
	err := os.WriteFile(testFile, []byte("initial"), 0o644)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store, err := New(logr.Discard(), ctx)
	require.NoError(t, err)

	// Add the file to the store
	err = store.AddFile(testFile)
	require.NoError(t, err)

	// Test concurrent read access
	done := make(chan bool, 4)

	// Counter to track successful operations
	readCount := int64(0)

	// Goroutine 1: Concurrent reads
	go func() {
		for i := 0; i < 200; i++ {
			content, exists := store.GetContent(testFile)
			if exists && len(content) > 0 {
				atomic.AddInt64(&readCount, 1)
			}
			time.Sleep(time.Microsecond * 100)
		}
		done <- true
	}()

	// Goroutine 2: More concurrent reads
	go func() {
		for i := 0; i < 200; i++ {
			content, exists := store.GetContent(testFile)
			if exists && len(content) > 0 {
				atomic.AddInt64(&readCount, 1)
			}
			time.Sleep(time.Microsecond * 100)
		}
		done <- true
	}()

	// Goroutine 3: Concurrent file updates (via filesystem)
	go func() {
		for i := 0; i < 100; i++ {
			content := fmt.Sprintf("updated-%d", i)
			os.WriteFile(testFile, []byte(content), 0o644)
			time.Sleep(time.Millisecond * 2)
		}
		done <- true
	}()

	// Goroutine 4: More concurrent reads
	go func() {
		for i := 0; i < 200; i++ {
			content, exists := store.GetContent(testFile)
			if exists && len(content) > 0 {
				atomic.AddInt64(&readCount, 1)
			}
			time.Sleep(time.Microsecond * 100)
		}
		done <- true
	}()

	// Wait for all goroutines to complete
	<-done
	<-done
	<-done
	<-done

	// Test should complete without panics or race conditions
	t.Logf("Completed test with %d read operations", atomic.LoadInt64(&readCount))

	// Verify store is still functional after concurrent access
	content, exists := store.GetContent(testFile)
	require.True(t, exists)
	assert.NotEmpty(t, content)
}

func TestStore_RefreshDeletesNonExistentFiles(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	testContent := "test content"

	// Create test file
	err := os.WriteFile(testFile, []byte(testContent), 0o644)
	require.NoError(t, err)

	// Create store with fsnotify
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store, err := New(logr.Discard(), ctx)
	require.NoError(t, err)

	err = store.AddFile(testFile)
	require.NoError(t, err)

	// Verify file is in store
	_, exists := store.GetContent(testFile)
	assert.True(t, exists)

	// Delete the file from filesystem
	err = os.Remove(testFile)
	require.NoError(t, err)

	// Wait for fsnotify to remove the file from store
	assert.Eventually(t, func() bool {
		_, exists := store.GetContent(testFile)
		return !exists
	}, 200*time.Millisecond, 10*time.Millisecond)
}

func TestStore_RotationEvents(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	originalContent := "original"
	updatedContent := "updated"

	// Create test file
	err := os.WriteFile(testFile, []byte(originalContent), 0o644)
	require.NoError(t, err)

	// Create store with fsnotify
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store, err := New(logr.Discard(), ctx)
	require.NoError(t, err)

	// Get channels
	rotationEvents := store.RotationEvents()
	errorEvents := store.Errors()

	// Test adding file - no event should be generated for "added"
	err = store.AddFile(testFile)
	require.NoError(t, err)

	// Should NOT receive an "added" event (only "updated" and "removed" are sent)
	select {
	case event := <-rotationEvents:
		t.Fatalf("Unexpected rotation event for file addition: %+v", event)
	case <-time.After(50 * time.Millisecond):
		// Expected - no event for "added"
	}

	// Update file and check for rotation event
	time.Sleep(10 * time.Millisecond)
	err = os.WriteFile(testFile, []byte(updatedContent), 0o644)
	require.NoError(t, err)

	// Should receive "updated" event
	select {
	case event := <-rotationEvents:
		assert.Equal(t, testFile, event.Path)
		assert.Equal(t, OperationUpdated, event.Operation)
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Expected rotation event for file update")
	}

	// Wait a bit to ensure all fsnotify events are processed
	time.Sleep(50 * time.Millisecond)

	// Consume any additional events that might have been generated by the file write
	// (some filesystems generate multiple events for a single write)
	for {
		select {
		case <-rotationEvents:
			// Consume extra events
		default:
			// No more events, break out of loop
			goto checkExternalDelete
		}
	}

checkExternalDelete:
	// Delete file externally (not through store) and check for rotation event
	err = os.Remove(testFile)
	require.NoError(t, err)

	// Should receive "removed" event from fsnotify
	select {
	case event := <-rotationEvents:
		assert.Equal(t, testFile, event.Path)
		assert.Equal(t, OperationRemoved, event.Operation)
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Expected rotation event for external file deletion")
	}

	// Ensure no errors occurred
	select {
	case err := <-errorEvents:
		t.Fatalf("Unexpected error: %v", err)
	default:
		// No error, as expected
	}
}

func TestStore_FileDeletedExternally(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	testContent := "test content"

	// Create test file
	err := os.WriteFile(testFile, []byte(testContent), 0o644)
	require.NoError(t, err)

	// Create store with fsnotify
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store, err := New(logr.Discard(), ctx)
	require.NoError(t, err)

	// Get rotation events channel
	rotationEvents := store.RotationEvents()

	err = store.AddFile(testFile)
	require.NoError(t, err)

	// No "added" event should be generated (only "updated" and "removed")
	select {
	case event := <-rotationEvents:
		t.Fatalf("Unexpected rotation event for file addition: %+v", event)
	case <-time.After(50 * time.Millisecond):
		// Expected - no event for "added"
	}

	// Delete the file externally (not through store)
	err = os.Remove(testFile)
	require.NoError(t, err)

	// Should receive "removed" event from fsnotify
	select {
	case event := <-rotationEvents:
		assert.Equal(t, testFile, event.Path)
		assert.Equal(t, OperationRemoved, event.Operation)
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Expected rotation event for external file deletion")
	}

	// Verify file was removed from store
	assert.Eventually(t, func() bool {
		_, exists := store.GetContent(testFile)
		return !exists
	}, 200*time.Millisecond, 10*time.Millisecond)
}

func TestStore_ChannelAccessors(t *testing.T) {
	ctx := context.Background()
	store, err := New(logr.Discard(), ctx)
	require.NoError(t, err)

	// Test that channel accessors return non-nil channels
	rotationCh := store.RotationEvents()
	assert.NotNil(t, rotationCh)

	errorCh := store.Errors()
	assert.NotNil(t, errorCh)

	// Channels should be read-only (this is enforced by the type system)
	// Just verify we can select on them
	select {
	case <-rotationCh:
		// Channel is empty, as expected
	case <-errorCh:
		// Channel is empty, as expected
	default:
		// Expected behavior - no events yet
	}
}
