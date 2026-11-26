package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/marcuoli/go-document-docx/document"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

func TestReportOmitsStatusScreenTips(t *testing.T) {
	var report Document
	report.BookmarkNext("status")
	report.Heading("Status", 1)
	report.AddTable(document.Table{Headers: []string{"Result"}, Widths: []int{4000}, Rows: [][]document.Cell{
		{{Runs: []document.CellRun{{Text: "!", TextColor: "000000"}, {Text: "?", TextColor: "AF2900"}}, ScreenTip: "Ação necessária (校正) & investigação"}},
		{{Text: "See status", LinkTarget: "status", ScreenTip: "Existing navigation"}},
	}})
	source := report.Source(Options{ContentsTitle: "Contents"})
	var output bytes.Buffer
	if err := source.Write(t.Context(), &output); err != nil {
		t.Fatal(err)
	}
	pdf, err := api.ReadContext(t.Context(), bytes.NewReader(output.Bytes()), model.NewStatelessConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if err := pdf.EnsurePageCount(); err != nil {
		t.Fatal(err)
	}
	links := 0
	for i := 1; i <= pdf.PageCount; i++ {
		page, _, _, err := pdf.PageDict(t.Context(), i, false)
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
			kind := annotation.NameEntry("Subtype")
			if kind == nil || *kind != "Link" {
				t.Fatalf("expected only navigation links, got %v", annotation)
			}
			links++
		}
	}
	if links != 2 || strings.Contains(output.String(), "pdf-note.invalid") {
		t.Fatalf("got %d links, want only the contents and authored links", links)
	}
}
