package store

import (
	"context"
	"os"
	"path/filepath"
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

	// Create store
	store := New(logr.Discard())

	// Test adding file
	err = store.AddFile("test", testFile)
	require.NoError(t, err)

	// Verify file was added
	content, exists := store.GetContent("test")
	assert.True(t, exists)
	assert.Equal(t, testContent, string(content))

	// Test adding non-existent file
	err = store.AddFile("nonexistent", "/path/that/does/not/exist")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "file does not exist")
}

func TestStore_RemoveFile(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")

	err := os.WriteFile(testFile, []byte("content"), 0o644)
	require.NoError(t, err)

	store := New(logr.Discard())

	// Add and then remove file
	err = store.AddFile("test", testFile)
	require.NoError(t, err)

	store.RemoveFile("test")

	// Verify file was removed
	_, exists := store.GetContent("test")
	assert.False(t, exists)
}

func TestStore_GetFile(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	testContent := "test content"

	err := os.WriteFile(testFile, []byte(testContent), 0o644)
	require.NoError(t, err)

	store := New(logr.Discard())
	err = store.AddFile("test", testFile)
	require.NoError(t, err)

	// Test GetFile
	fileStore, exists := store.GetFile("test")
	require.True(t, exists)
	assert.Equal(t, testFile, fileStore.Path)
	assert.Equal(t, testContent, string(fileStore.Content))

	// Test non-existent file
	_, exists = store.GetFile("nonexistent")
	assert.False(t, exists)
}

func TestStore_PeriodicRefresh(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	originalContent := "original"
	updatedContent := "updated"

	// Create test file
	err := os.WriteFile(testFile, []byte(originalContent), 0o644)
	require.NoError(t, err)

	store := New(logr.Discard())
	err = store.AddFile("test", testFile)
	require.NoError(t, err)

	// Start periodic refresh with short interval
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store.StartPeriodicRefresh(ctx, 50*time.Millisecond)

	// Wait a bit, then update file
	time.Sleep(10 * time.Millisecond)
	err = os.WriteFile(testFile, []byte(updatedContent), 0o644)
	require.NoError(t, err)

	// Wait for periodic refresh to pick up the change
	assert.Eventually(t, func() bool {
		content, exists := store.GetContent("test")
		return exists && string(content) == updatedContent
	}, 200*time.Millisecond, 10*time.Millisecond)
}

func TestStore_ConcurrentAccess(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")

	err := os.WriteFile(testFile, []byte("initial"), 0o644)
	require.NoError(t, err)

	store := New(logr.Discard())
	err = store.AddFile("test", testFile)
	require.NoError(t, err)

	// Test concurrent read/write access
	done := make(chan bool, 2)

	// Goroutine 1: Keep reading
	go func() {
		for i := 0; i < 100; i++ {
			store.GetContent("test")
			time.Sleep(time.Millisecond)
		}
		done <- true
	}()

	// Goroutine 2: Keep reading (testing concurrent reads)
	go func() {
		for i := 0; i < 100; i++ {
			store.GetFile("test")
			time.Sleep(time.Millisecond)
		}
		done <- true
	}()

	// Wait for both goroutines to complete
	<-done
	<-done

	// Should not panic or cause race conditions
	content, exists := store.GetContent("test")
	assert.True(t, exists)
	assert.NotEmpty(t, content)
}
