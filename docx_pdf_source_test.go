package pdf

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"github.com/marcuoli/go-document-docx/document"
	"github.com/marcuoli/go-document-docx/report"
)

func TestPDFStageUnwrapsOnlyGeneratedTips(t *testing.T) {
	doc, err := document.New(report.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	doc.BookmarkNext("heading")
	doc.Heading("Title", 1)
	doc.AddTable(document.Table{Headers: []string{"Result"}, Widths: []int{4000}, Rows: [][]document.Cell{
		{{Runs: []document.CellRun{{Text: "✔", TextColor: "000000"}, {Text: " ⚠️", TextColor: "F2A100", Italic: true}}, ScreenTip: "Ação & note"}},
		{{Text: "Jump", LinkTarget: "heading", ScreenTip: "Navigate"}},
		{{Text: "Explicit", Bookmark: "_DocxCellTip55", LinkTarget: "_DocxCellTip55", ScreenTip: "Authored self-link"}},
	}})
	var buf bytes.Buffer
	if err := doc.Write(t.Context(), &buf); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	body, err := stripGeneratedScreenTips(t.Context(), reader)
	if err != nil {
		t.Fatal(err)
	}
	xml := string(body)
	if strings.Count(xml, "<w:hyperlink ") != 2 || strings.Contains(xml, "Ação") ||
		!strings.Contains(xml, `w:tooltip="Navigate"`) ||
		!strings.Contains(xml, `w:tooltip="Authored self-link"`) ||
		!strings.Contains(xml, `w:anchor="heading"`) ||
		!strings.Contains(xml, `w:anchor="_DocxCellTip55"`) {
		t.Fatal("expected only the automatically generated ScreenTip to be unwrapped")
	}
	if !strings.Contains(xml, ">✔</w:t>") || !strings.Contains(xml, "> ⚠️</w:t>") ||
		!strings.Contains(xml, `<w:color w:val="F2A100"`) || !strings.Contains(xml, "<w:i") {
		t.Fatal("unwrapping the generated hyperlink changed its status runs")
	}
}
