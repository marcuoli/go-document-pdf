package report

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/marcuoli/go-document-docx/document"
)

func TestSourceRendersStructuredReport(t *testing.T) {
	var d Document
	d.BookmarkNext("Review")
	d.Heading("Assessment & evidence", 1)
	d.ParagraphWithNext("Recorded <value>")
	d.AddTable(document.Table{
		Headers: []string{"Property", "Result"}, Widths: []int{3500, 2500},
		Rows: [][]document.Cell{{{Text: `<img src="file:///private"> & missing`}, {
			LinkTarget: "Review", Runs: []document.CellRun{{Text: "Review"}, {Text: "!", TextColor: "FFB000"}},
		}}},
	})
	d.CodeTable("Command", "check 'a < b'\nstatus")
	markup := d.body.String()
	for _, want := range []string{
		`<h1 id="review">Assessment &amp; evidence</h1>`, "Recorded &lt;value&gt;",
		"&lt;img src=", `href="#review"`, "check &#39;a &lt; b&#39;<br>status",
		`<th style="width:58.3333%">`, `<th style="width:41.6667%">`,
	} {
		if !strings.Contains(markup, want) {
			t.Errorf("report omitted %q", want)
		}
	}
	if strings.Contains(markup, "<img") {
		t.Fatal("evidence was interpreted as markup")
	}

	art := syntheticPNG(t)
	source := d.Source(Options{
		Title: "Assessment", ContentsTitle: "Contents",
		Cover:          &Cover{Subtitle: "Subject", LogoPNG: art, TrimPNG: art},
		HeaderImagePNG: art,
		Footer:         &Footer{Label: "Assessment", Text: "Revision 1", LogoPNG: art},
	})
	// A snapshot owns the printable input even if the builder or the caller's
	// artwork buffer is reused before writing.
	clear(art)
	d.AddTable(document.Table{})
	var pdf bytes.Buffer
	if err := source.Write(t.Context(), &pdf); err != nil {
		t.Fatal(err)
	}
	images := bytes.Count(pdf.Bytes(), []byte("/Subtype /Image"))
	links := bytes.Count(pdf.Bytes(), []byte("/Subtype /Link"))
	if !bytes.HasPrefix(pdf.Bytes(), []byte("%PDF-")) || !bytes.Contains(pdf.Bytes(), []byte("/Outlines")) ||
		links < 2 || images < 4 {
		t.Fatalf("portable report lost its PDF outline, navigation or artwork (%d links, %d images)", links, images)
	}
}

func TestInvalidTableLeavesOutputUntouched(t *testing.T) {
	for _, table := range []document.Table{
		{},
		{Headers: []string{"Name"}, Widths: []int{0}},
		{Headers: []string{"Name"}, Widths: []int{1000}, Rows: [][]document.Cell{{}}},
	} {
		var d Document
		d.AddTable(table)
		source := d.Source(Options{})
		var output bytes.Buffer
		if err := source.Write(t.Context(), &output); err == nil || output.Len() != 0 {
			t.Fatalf("invalid PDF table wrote %d bytes, err %v", output.Len(), err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if err := source.Write(ctx, &output); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled source returned %v", err)
		}
	}
}

func syntheticPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 12, 12))
	img.Set(3, 3, color.RGBA{R: 100, G: 50, B: 25, A: 255})
	var pngBytes bytes.Buffer
	if err := png.Encode(&pngBytes, img); err != nil {
		t.Fatal(err)
	}
	return pngBytes.Bytes()
}
