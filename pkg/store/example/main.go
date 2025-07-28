package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Azure/aks-app-routing-operator/pkg/store"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func main() {
	// Initialize logger
	logger := zap.New(zap.UseDevMode(true))

	// Create a new file store
	fileStore := store.New(logger)

	// Create some example files to track
	tempDir, err := os.MkdirTemp("", "store-example")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tempDir)

	// Create example configuration files
	configFile := filepath.Join(tempDir, "config.yaml")
	secretFile := filepath.Join(tempDir, "secret.txt")

	err = os.WriteFile(configFile, []byte(`
apiVersion: v1
kind: Config
data:
  setting1: value1
  setting2: value2
`), 0o644)
	if err != nil {
		panic(err)
	}

	err = os.WriteFile(secretFile, []byte("secret-token-12345"), 0o644)
	if err != nil {
		panic(err)
	}

	// Add files to the store
	fmt.Println("Adding files to store...")
	if err := fileStore.AddFile("config", configFile); err != nil {
		panic(err)
	}

	if err := fileStore.AddFile("secret", secretFile); err != nil {
		panic(err)
	}

	// Read initial content
	fmt.Println("\nInitial file contents:")
	if content, exists := fileStore.GetContent("config"); exists {
		fmt.Printf("Config file:\n%s\n", string(content))
	}

	if content, exists := fileStore.GetContent("secret"); exists {
		fmt.Printf("Secret file: %s\n", string(content))
	}

	// Demonstrate file metadata
	fmt.Println("\nFile metadata:")
	if file, exists := fileStore.GetFile("config"); exists {
		fmt.Printf("Config - Path: %s, Size: %d bytes\n",
			file.Path, len(file.Content))
	}

	// Start periodic refresh (every 2 seconds)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fmt.Println("\nStarting periodic refresh (every 2 seconds)...")
	fileStore.StartPeriodicRefresh(ctx, 2*time.Second)

	// Simulate file changes
	go func() {
		time.Sleep(3 * time.Second)
		fmt.Println("\nUpdating config file...")
		updatedConfig := `
apiVersion: v1
kind: Config
data:
  setting1: updated-value1
  setting2: updated-value2
  setting3: new-value3
`
		if err := os.WriteFile(configFile, []byte(updatedConfig), 0o644); err != nil {
			fmt.Printf("Error updating config: %v\n", err)
		}

		time.Sleep(2 * time.Second)
		fmt.Println("\nUpdating secret file...")
		if err := os.WriteFile(secretFile, []byte("new-secret-token-67890"), 0o644); err != nil {
			fmt.Printf("Error updating secret: %v\n", err)
		}
	}()

	// Monitor changes for 10 seconds
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	startTime := time.Now()
	for {
		select {
		case <-ticker.C:
			if time.Since(startTime) > 10*time.Second {
				fmt.Println("\nStopping monitoring...")
				return
			}

			// Check current content periodically
			if content, exists := fileStore.GetContent("config"); exists {
				fmt.Printf("Config size: %d bytes\n", len(content))
			}

		case <-ctx.Done():
			return
		}
	}
}
