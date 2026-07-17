//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	fmt.Println("Usage: go run run.go run_commands.go [command]")
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
	return execCmd("go", "build", "-o", "audiobookshelf-go", "main.go", "config.go", "version.go")
}
