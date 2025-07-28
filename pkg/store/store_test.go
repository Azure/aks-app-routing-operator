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

	// Create store without periodic refresh
	ctx := context.Background()
	store := New(logr.Discard(), ctx, 0) // 0 interval disables periodic refresh

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

	ctx := context.Background()
	store := New(logr.Discard(), ctx, 0) // 0 interval disables periodic refresh

	// Add and then remove file
	err = store.AddFile("test", testFile)
	require.NoError(t, err)

	store.RemoveFile("test")

	// Verify file was removed
	_, exists := store.GetContent("test")
	assert.False(t, exists)
}

func TestStore_GetContent(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	testContent := "test content"

	err := os.WriteFile(testFile, []byte(testContent), 0o644)
	require.NoError(t, err)

	ctx := context.Background()
	store := New(logr.Discard(), ctx, 0) // 0 interval disables periodic refresh
	err = store.AddFile("test", testFile)
	require.NoError(t, err)

	// Test GetContent
	content, exists := store.GetContent("test")
	require.True(t, exists)
	assert.Equal(t, testContent, string(content))

	// Test non-existent file
	_, exists = store.GetContent("nonexistent")
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

	// Start periodic refresh with short interval
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := New(logr.Discard(), ctx, 50*time.Millisecond)
	err = store.AddFile("test", testFile)
	require.NoError(t, err)

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

	ctx := context.Background()
	store := New(logr.Discard(), ctx, 0) // 0 interval disables periodic refresh
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
			store.GetContent("test")
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

func TestStore_RefreshDeletesNonExistentFiles(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	testContent := "test content"

	// Create test file
	err := os.WriteFile(testFile, []byte(testContent), 0o644)
	require.NoError(t, err)

	// Start periodic refresh with short interval
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := New(logr.Discard(), ctx, 50*time.Millisecond)
	err = store.AddFile("test", testFile)
	require.NoError(t, err)

	// Verify file is in store
	_, exists := store.GetContent("test")
	assert.True(t, exists)

	// Delete the file from filesystem
	err = os.Remove(testFile)
	require.NoError(t, err)

	// Wait for periodic refresh to remove the file from store
	assert.Eventually(t, func() bool {
		_, exists := store.GetContent("test")
		return !exists
	}, 200*time.Millisecond, 10*time.Millisecond)
}
