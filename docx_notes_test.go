package pdf

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"github.com/marcuoli/go-document-docx/document"
	"github.com/marcuoli/go-document-docx/report"
)

func TestWordNoteStageRewritesOnlyGeneratedTips(t *testing.T) {
	doc, err := document.New(report.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	doc.BookmarkNext("heading")
	doc.Heading("Title", 1)
	doc.AddTable(document.Table{Headers: []string{"Result"}, Widths: []int{4000}, Rows: [][]document.Cell{
		{{Text: "!", ScreenTip: "Ação & note"}},
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
	parts, _, err := wordNoteParts(t.Context(), reader)
	if err != nil {
		t.Fatal(err)
	}
	body, rels := string(parts[documentPart]), string(parts[relationshipsPart])
	if strings.Count(rels, "https://pdf-note.invalid/") != 1 || strings.Count(body, "noteRel:id=") != 1 ||
		strings.Contains(body, "Ação") || !strings.Contains(body, "Navigate") || !strings.Contains(body, "Authored self-link") {
		t.Fatal("expected only the automatically generated ScreenTip to move to a private marker")
	}
}
