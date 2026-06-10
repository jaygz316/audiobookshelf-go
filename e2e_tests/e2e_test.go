package e2e_tests

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	code := m.Run()
	CleanupTestBinary()
	os.Exit(code)
}

