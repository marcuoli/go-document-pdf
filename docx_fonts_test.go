package pdf

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/marcuoli/go-document-docx/publication"
	"github.com/marcuoli/go-document-pdf/internal/reportfonts"
)

func TestSavePortableFromDOCXUsesVerifiedFontsAndPreservesExistingPDF(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	input, output := filepath.Join(dir, "report.docx"), filepath.Join(dir, "report.pdf")
	var docx bytes.Buffer
	if err := zip.NewWriter(&docx).Close(); err != nil {
		t.Fatal(err)
	}
	receipt, err := publication.Save(t.Context(), input, false, publication.FromBytes(docx.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	portable := sourceFunc(func(ctx context.Context, w io.Writer) error {
		return reportfonts.Render(ctx, func(reportfonts.Faces) error {
			_, err := io.WriteString(w, "portable PDF")
			return err
		})
	})
	published, err := SavePortableFromDOCX(t.Context(), output, input, receipt, portable, false)
	if err != nil || !published.Installed || published.Backend != Portable {
		t.Fatalf("portable install: %+v, %v", published, err)
	}

	// A same-size substitution must fail during font staging, before render.
	if err := os.WriteFile(input, bytes.Repeat([]byte{'x'}, docx.Len()), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := SavePortableFromDOCX(t.Context(), output, input, receipt, portable, true)
	if !errors.Is(err, errDocumentChanged) || result.Installed {
		t.Fatalf("modified DOCX was admitted: %+v, %v", result, err)
	}
	got, err := os.ReadFile(output)
	if err != nil || string(got) != "portable PDF" {
		t.Fatalf("previous PDF changed: %q, %v", got, err)
	}
}

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
