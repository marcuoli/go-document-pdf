package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/marcuoli/go-document-docx/document"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

func TestReportScreenTipNotes(t *testing.T) {
	var report Document
	report.BookmarkNext("status")
	report.Heading("Status", 1)
	const explanation = "Ação necessária (校正) & investigação"
	report.AddTable(document.Table{Headers: []string{"Result"}, Widths: []int{4000}, Rows: [][]document.Cell{
		{{Runs: []document.CellRun{{Text: "!", TextColor: "000000"}, {Text: "?", TextColor: "AF2900"}}, ScreenTip: explanation}},
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
	var notes, links int
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
			if kind := annotation.NameEntry("Subtype"); kind != nil {
				switch *kind {
				case "Square":
					notes++
					text, err := annotation.StringOrHexLiteralEntry("Contents")
					appearance, appearanceErr := pdf.DereferenceDict(annotation["AP"])
					if err != nil || text == nil || *text != explanation || annotation["A"] != nil || appearanceErr != nil || appearance["N"] == nil {
						t.Fatalf("wrong note contents or action: %v, %v", text, err)
					}
				case "Link":
					links++
				}
			}
		}
	}
	if notes != 1 || links < 2 || strings.Contains(output.String(), "pdf-note.invalid") {
		t.Fatalf("notes=%d, regular links=%d; expected one note and retained navigation with no marker URL", notes, links)
	}
}
