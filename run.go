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

func main() {
	if len(os.Args) < 2 {
		// Default action is build
		if err := runBuild(); err != nil {
			fmt.Fprintf(os.Stderr, "Build failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	command := os.Args[1]
	var err error

	switch command {
	case "build":
		err = runBuild()
	case "test":
		err = runTest()
	case "vet":
		err = runVet()
	case "fmt":
		err = runFmt()
	case "fmt-check":
		err = runFmtCheck()
	case "setup":
		err = runSetup()
	case "fe-build":
		err = runFrontendBuild()
	case "docker-build":
		err = runDockerBuild()
	case "docker-push":
		err = runDockerPush()
	case "clean":
		err = runClean()
	case "start":
		err = runStart()
	case "help", "-h", "--help":
		printHelp()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		printHelp()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Command %q failed: %v\n", command, err)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println("Usage: go run run.go [command]")
	fmt.Println("\nAvailable commands:")
	fmt.Println("  build        Build the server binary (default)")
	fmt.Println("  test         Run all tests")
	fmt.Println("  vet          Run go vet")
	fmt.Println("  fmt          Format all Go source files")
	fmt.Println("  fmt-check    Verify all Go source files are formatted")
	fmt.Println("  setup        Configure git directory safety and download Go modules")
	fmt.Println("  fe-build     Install frontend dependencies and generate static Nuxt SPA assets")
	fmt.Println("  docker-build Build docker image")
	fmt.Println("  docker-push  Push docker image to Docker Hub")
	fmt.Println("  clean        Remove compiled binaries")
	fmt.Println("  start        Start the development server with ROUTER_BASE_PATH=\"/\"")
	fmt.Println("  help         Show this help message")
}

// execCmd runs a command forwarding stdin, stdout, and stderr.
func execCmd(name string, args ...string) error {
	return execCmdWithEnv(nil, name, args...)
}

func execCmdWithEnv(env []string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
	return cmd.Run()
}

func copyWasmExec() error {
	src := "/usr/lib/golang/lib/wasm/wasm_exec.js"
	dst := filepath.Join("frontend", "js", "wasm_exec.js")
	fmt.Printf("Copying wasm_exec.js from %s to %s...\n", src, dst)

	content, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("failed to read wasm_exec.js: %w", err)
	}

	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(dst, content, 0644); err != nil {
		return fmt.Errorf("failed to write wasm_exec.js: %w", err)
	}
	return nil
}

func compileWasm() error {
	if err := copyWasmExec(); err != nil {
		return err
	}
	fmt.Println("Building Go WebAssembly frontend...")
	cmd := exec.Command("go", "build", "-o", filepath.Join("frontend", "main.wasm"), "./frontend/go")
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runBuild() error {
	if err := compileWasm(); err != nil {
		return err
	}
	fmt.Println("Building audiobookshelf-go...")
	return execCmd("go", "build", "-o", "audiobookshelf-go", ".")
}

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
	return execCmdWithEnv(env, "go", "run", "main.go")
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
