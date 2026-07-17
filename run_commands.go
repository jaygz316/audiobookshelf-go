//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"go/format"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func runTest() error {
	fmt.Println("Running tests...")
	return execCmd("go", "test", "-v", ".", "./internal/...", "./e2e/...")
}

func runVet() error {
	fmt.Println("Running go vet on native packages...")
	if err := execCmd("go", "vet", ".", "./internal/...", "./e2e/..."); err != nil {
		return err
	}
	fmt.Println("Running go vet on WebAssembly package...")
	cmd := exec.Command("go", "vet", "./frontend/go")
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runFmt() error {
	fmt.Println("Formatting Go files...")
	return execCmd("go", "fmt", "./...")
}

func runFmtCheck() error {
	fmt.Println("Checking Go file formatting...")
	var unformattedFiles []string

	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if path == "." {
				return nil
			}
			// Skip directories that we don't want to traverse
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "frontend" || name == "metadata" || name == "config" || name == "scratch" {
				return filepath.SkipDir
			}
			return nil
		}

		if filepath.Ext(path) == ".go" {
			content, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("read file %s: %w", path, err)
			}

			formatted, err := format.Source(content)
			if err != nil {
				return fmt.Errorf("format file %s: %w", path, err)
			}

			if string(content) != string(formatted) {
				unformattedFiles = append(unformattedFiles, path)
			}
		}
		return nil
	})

	if err != nil {
		return err
	}

	if len(unformattedFiles) > 0 {
		fmt.Fprintln(os.Stderr, "The following Go files are not formatted:")
		for _, file := range unformattedFiles {
			fmt.Fprintf(os.Stderr, "  - %s\n", file)
		}
		fmt.Fprintln(os.Stderr, "Please run 'go run run.go fmt' or 'make fmt' to fix them.")
		return fmt.Errorf("found %d unformatted file(s)", len(unformattedFiles))
	}

	fmt.Println("All Go files are properly formatted.")
	return nil
}

func runDockerBuild() error {
	fmt.Println("Building Docker image...")
	return execCmd("docker", "build", "-t", "jaygz/audiobookshelf-go:latest", ".")
}

func runDockerPush() error {
	fmt.Println("Pushing Docker image to Docker Hub...")
	return execCmd("docker", "push", "jaygz/audiobookshelf-go:latest")
}

func runClean() error {
	fmt.Println("Cleaning build outputs...")
	// Remove both potential binary names
	err1 := os.Remove("audiobookshelf-go")
	if err1 != nil && !os.IsNotExist(err1) {
		fmt.Fprintf(os.Stderr, "Warning: failed to remove audiobookshelf-go: %v\n", err1)
	}
	err2 := os.Remove("audiobookshelf")
	if err2 != nil && !os.IsNotExist(err2) {
		fmt.Fprintf(os.Stderr, "Warning: failed to remove audiobookshelf: %v\n", err2)
	}
	return nil
}

func runStart() error {
	fmt.Println("Starting audiobookshelf-go development server...")
	env := []string{
		"ROUTER_BASE_PATH=/",
	}
	return execCmdWithEnv(env, "go", "run", "main.go", "config.go", "version.go")
}

func runSetup() error {
	fmt.Println("Running post-create setup...")
	pwd, err := os.Getwd()
	if err != nil {
		return err
	}
	fmt.Printf("Configuring git safe.directory for %s...\n", pwd)
	if err := execCmd("git", "config", "--global", "--add", "safe.directory", pwd); err != nil {
		return fmt.Errorf("git config failed: %w", err)
	}
	fmt.Println("Downloading Go modules...")
	if err := execCmd("go", "mod", "download"); err != nil {
		return fmt.Errorf("go mod download failed: %w", err)
	}
	fmt.Println("Setup completed successfully.")
	return nil
}

func runFrontendBuild() error {
	return compileWasm()
}
