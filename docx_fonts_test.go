package pdf

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/marcuoli/go-document-docx/publication"
	"github.com/marcuoli/go-document-pdf/internal/reportfonts"
)

func TestPortableFontStageUsesVerifiedDOCXAndCleansUp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "final.docx")
	var empty bytes.Buffer
	if err := zip.NewWriter(&empty).Close(); err != nil {
		t.Fatal(err)
	}
	receipt, err := publication.Save(t.Context(), path, false, publication.FromBytes(empty.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	source := portableDOCX{document: finalizedDOCX{path, receipt}, directory: dir}
	called := false
	want := errors.New("render failure")
	if err := source.withFonts(t.Context(), func(faces reportfonts.Faces) error {
		called = true
		if faces != (reportfonts.Faces{}) {
			t.Fatalf("an empty package supplied faces: %+v", faces)
		}
		return want
	}); !called || !errors.Is(err, want) {
		t.Fatalf("render failure not propagated: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "final.docx" {
		t.Fatalf("portable font stage leaked: %v, %v", entries, err)
	}
	if err := os.WriteFile(path, bytes.Repeat([]byte{'x'}, empty.Len()), 0600); err != nil {
		t.Fatal(err)
	}
	if err := source.withFonts(t.Context(), func(reportfonts.Faces) error {
		t.Fatal("render accepted a replaced DOCX")
		return nil
	}); !errors.Is(err, errDocumentChanged) {
		t.Fatalf("unexpected replacement result: %v", err)
	}
}
