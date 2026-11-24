package pdf

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/marcuoli/go-document-docx/publication"
)

func TestFinalizedDOCXRequiresThePublishedBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.docx")
	original := []byte("original")
	receipt, err := publication.Save(t.Context(), path, false, publication.FromBytes(original))
	if err != nil || !receipt.Installed {
		t.Fatalf("install test document: %+v, %v", receipt, err)
	}
	source := finalizedDOCX{path, receipt}
	var staged bytes.Buffer
	if err := source.Write(t.Context(), &staged); err != nil || !bytes.Equal(staged.Bytes(), original) {
		t.Fatalf("stage original: %v, %q", err, staged.Bytes())
	}
	if err := os.WriteFile(path, []byte("replaced"), 0600); err != nil {
		t.Fatal(err)
	}
	// Matching length and pathname do not make this the installed artifact.
	staged.Reset()
	if err := source.Write(t.Context(), &staged); !errors.Is(err, errDocumentChanged) {
		t.Fatalf("same-size replacement reached export: %v", err)
	}
}

func TestSaveFromDOCXRejectsTheInputAsOutput(t *testing.T) {
	input := filepath.Join(t.TempDir(), "report.docx")
	original := []byte("published document")
	receipt, err := publication.Save(t.Context(), input, false, publication.FromBytes(original))
	if err != nil {
		t.Fatal(err)
	}
	replacement := publication.FromBytes([]byte("replacement"))
	result, err := SaveFromDOCX(t.Context(), input, input, receipt, replacement, true)
	if err == nil || result.Installed {
		t.Fatal("PDF publisher accepted the input as its destination")
	}
	got, err := os.ReadFile(input)
	if err != nil || !bytes.Equal(got, original) {
		t.Fatalf("original document changed: %q, %v", got, err)
	}
}

func TestSaveFromDOCXRejectsAliasedDestination(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	if err := os.Mkdir(real, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(real, alias); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}
	path := filepath.Join(real, "report.docx")
	original := []byte("published document")
	receipt, err := publication.Save(t.Context(), path, false, publication.FromBytes(original))
	if err != nil {
		t.Fatal(err)
	}
	replacement := publication.FromBytes([]byte("replacement"))
	result, err := SaveFromDOCX(t.Context(), filepath.Join(alias, "report.docx"), path, receipt, replacement, true)
	if !errors.Is(err, errDOCXDestination) || result.Installed {
		t.Fatalf("aliased output was not rejected: %+v, %v", result, err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, original) {
		t.Fatalf("original document changed: %q, %v", got, err)
	}
}
