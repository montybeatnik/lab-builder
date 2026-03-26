package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizeLabPath(t *testing.T) {
	base := t.TempDir()
	cfg := Config{BaseDir: base}

	if _, err := cfg.SanitizeLabPath(""); err == nil {
		t.Fatal("expected error for empty path")
	}

	filePath := filepath.Join(base, "lab.clab.yml")
	if err := os.WriteFile(filePath, []byte("test"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	abs, err := cfg.SanitizeLabPath("lab.clab.yml")
	if err != nil {
		t.Fatalf("sanitize relative file: %v", err)
	}
	if abs != filePath {
		t.Fatalf("expected %q got %q", filePath, abs)
	}

	dir := filepath.Join(base, "labdir")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	fileInDir := filepath.Join(dir, "lab.clab.yml")
	if err := os.WriteFile(fileInDir, []byte("test"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	abs, err = cfg.SanitizeLabPath("labdir")
	if err != nil {
		t.Fatalf("sanitize dir: %v", err)
	}
	if abs != fileInDir {
		t.Fatalf("expected %q got %q", fileInDir, abs)
	}

	outside := filepath.Join(t.TempDir(), "lab.clab.yml")
	if err := os.WriteFile(outside, []byte("test"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := cfg.SanitizeLabPath(outside); err == nil {
		t.Fatal("expected error for path outside basedir")
	}
}
