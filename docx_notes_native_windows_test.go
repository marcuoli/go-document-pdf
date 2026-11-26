//go:build windows && integration

package pdf

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/marcuoli/go-document-docx/document"
	"github.com/marcuoli/go-document-docx/publication"
	"github.com/marcuoli/go-document-docx/report"
	"github.com/marcuoli/go-document-docx/word"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

func TestNativeWordNotesAfterFieldRefresh(t *testing.T) {
	if os.Getenv("DOCX_WORD_INTEGRATION") != "1" {
		t.Skip("explicit desktop Word integration opt-in required")
	}
	ctx := t.Context()
	dir := t.TempDir()
	doc, err := document.New(report.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	doc.Heading("Status", 1)
	const tip = "Ação necessária"
	doc.AddTable(document.Table{Headers: []string{"Result"}, Widths: []int{4000}, Rows: [][]document.Cell{
		{{Text: "Review", ScreenTip: tip}},
	}})
	input, output := filepath.Join(dir, "source.docx"), filepath.Join(dir, "report.pdf")
	receipt, err := publication.Save(ctx, input, false, word.WithUpdatedFields(doc, word.Options{TemporaryDirectory: dir}))
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	result, err := SaveFromDOCX(ctx, output, input, receipt, nil, false)
	if err != nil || result.Backend != Word || !result.Installed {
		t.Fatalf("expected native Word PDF: %+v, %v", result, err)
	}
	after, err := os.ReadFile(input)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("the installed DOCX changed", err)
	}
	file, err := os.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	pdf, err := api.ReadContext(ctx, file, model.NewStatelessConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if err := pdf.EnsurePageCount(); err != nil {
		t.Fatal(err)
	}
	count := 0
	for i := 1; i <= pdf.PageCount; i++ {
		page, _, _, err := pdf.PageDict(ctx, i, false)
		if err != nil {
			t.Fatal(err)
		}
		annotations, err := pdf.DereferenceArray(page["Annots"])
		if err != nil {
			t.Fatal(err)
		}
		for _, ref := range annotations {
			annotation, err := pdf.DereferenceDict(ref)
			if err != nil {
				t.Fatal(err)
			}
			if kind := annotation.NameEntry("Subtype"); kind != nil && *kind == "Text" {
				text, err := annotation.StringOrHexLiteralEntry("Contents")
				if err != nil || text == nil || *text != tip {
					t.Fatal("native note did not keep the original explanation", err)
				}
				count++
			}
		}
	}
	if count != 1 {
		t.Fatalf("native Word emitted %d notes, want one after field refresh", count)
	}
}
