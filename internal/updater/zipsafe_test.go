package updater

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSafeZipPathRejectsTraversal pins the v1.1.4Z zip-slip fix: the
// updater's extractZip previously joined member names with no validation.
func TestSafeZipPathRejectsTraversal(t *testing.T) {
	dir := t.TempDir()

	blocked := []string{
		"../../outside.exe",
		"../outside.exe",
		"/absolute/path",
		`C:\outside.exe`,
		`\\server\share\file`,
		`bin/../../escape`,
	}

	for _, name := range blocked {
		if _, err := safeZipPath(dir, name); err == nil {
			t.Errorf("member %q must be rejected", name)
		}
	}

	allowed := []string{
		"llama-server.exe",
		"bin/llama-server.exe",
		`sub\dir\dll.dll`,
	}

	for _, name := range allowed {
		target, err := safeZipPath(dir, name)
		if err != nil {
			t.Errorf("member %q must be accepted: %v", name, err)
			continue
		}

		rel, err := filepath.Rel(dir, target)
		if err != nil || strings.HasPrefix(rel, "..") {
			t.Errorf("member %q resolved outside dir: %s", name, target)
		}
	}
}

// TestExtractZipRefusesEscapingMembers builds a hostile zip and proves
// extraction refuses to write outside the target directory.
func TestExtractZipRefusesEscapingMembers(t *testing.T) {
	outside := t.TempDir()
	inside := filepath.Join(outside, "stage")

	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	zipPath := filepath.Join(outside, "evil.zip")

	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	zw := zip.NewWriter(f)
	w, err := zw.Create("../../escaped.txt")
	if err != nil {
		t.Fatalf("zip create: %v", err)
	}
	w.Write([]byte("payload"))
	zw.Close()
	f.Close()

	if err := extractZip(zipPath, inside); err == nil {
		t.Fatal("extractZip must reject a traversal member")
	}

	if _, err := os.Stat(filepath.Join(outside, "escaped.txt")); err == nil {
		t.Fatal("payload escaped the staging directory — zip-slip regression")
	}
}
