package report

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

func TestSectionHTMLPreservesTextAndOnlyLinksExternalAddresses(t *testing.T) {
	markup, _ := sectionHTML([]Section{{Heading: "A & B", Paragraphs: [][]TextRun{
		{{Text: "<p>\n"}, {Text: "website", URL: "https://example.org/?a=1&b=2"}},
		{{Text: "Contact", URL: "mailto:office@example.org"}, {Text: "Unsafe", URL: "javascript:alert(1)"}},
		{{Text: "Bad host", URL: "https:/example.org"}, {Text: "Local file", URL: "file:///private"}},
	}}}, "")
	for _, want := range []string{
		"<h2>A &amp; B</h2>", "<p>&lt;p&gt;<br>",
		`href="https://example.org/?a=1&amp;b=2">website</a>`,
		`href="mailto:office@example.org">Contact</a>Unsafe`, "<p>Bad hostLocal file</p>",
	} {
		if !strings.Contains(markup, want) {
			t.Errorf("section omitted %q: %s", want, markup)
		}
	}
	if strings.Count(markup, "<a ") != 2 {
		t.Fatalf("section linked unusable URLs: %s", markup)
	}
}

func TestSourcePaginatesOpeningContentsBodyAndClosingWithNavigation(t *testing.T) {
	var body Document
	body.PageBreak()
	body.BookmarkNext("Report")
	body.Heading("Report", 1)
	body.Paragraph("A short report")
	body.PageBreak()
	body.PageBreak()
	body.Heading("Pair summary", 1)
	body.Paragraph("Pair details")
	body.PageBreak()
	source := body.Source(Options{
		ContentsTitle: "Contents",
		Opening:       []Section{{Heading: "Opening", Paragraphs: [][]TextRun{{{Text: "Opening text"}}}}},
		Closing:       []Section{{Heading: "Closing", Paragraphs: [][]TextRun{{{Text: "Contact", URL: "mailto:office@example.org"}}}}},
	})
	var pdf bytes.Buffer
	if err := source.Write(t.Context(), &pdf); err != nil {
		t.Fatal(err)
	}
	pageCount := len(regexp.MustCompile(`/Type\s*/Page\b`).FindAll(pdf.Bytes(), -1))
	if pageCount != 5 || bytes.Count(pdf.Bytes(), []byte("/Subtype /Link")) != 4 ||
		!bytes.Contains(pdf.Bytes(), []byte("mailto:office@example.org")) {
		t.Fatalf("expected opening, linked contents, two body pages, and linked closing (got %d pages)", pageCount)
	}
}
