// Package report builds a portable PDF from structured report blocks.
package report

import (
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/marcuoli/go-document-docx/document"
	"github.com/marcuoli/go-document-pdf/internal/notes"
)

// Document accumulates report blocks. Its zero value is ready to use. The
// builder is for a single writer. Source takes a snapshot for later rendering.
type Document struct {
	headings  []heading
	body      strings.Builder
	bodyParts []string
	bookmarks []string
	notes     notes.Set
	err       error
}

type heading struct {
	text, id string
	level    int
}

func textHTML(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return reportMarks.Replace(strings.ReplaceAll(html.EscapeString(text), "\n", "<br>"))
}

func anchor(name string) string { return html.EscapeString(strings.ToLower(name)) }

// BookmarkNext attaches a case-insensitive internal bookmark to the next
// heading or paragraph. Empty names are ignored.
func (d *Document) BookmarkNext(name string) {
	if name != "" {
		d.bookmarks = append(d.bookmarks, name)
	}
}

// Heading appends a level 1 to 6 heading and adds it to the document outline.
func (d *Document) Heading(text string, level int) {
	level = min(6, max(1, level))
	id := fmt.Sprintf("portable-section-%d", len(d.headings)+1)
	if len(d.bookmarks) == 0 {
		d.BookmarkNext(id)
	} else {
		id = d.bookmarks[0]
	}
	d.headings = append(d.headings, heading{text, id, level})
	d.paragraph("h"+strconv.Itoa(level), "", text)
}

// PageBreak starts the next report block on a fresh page. Repeated breaks
// and a break after the final block do not produce empty pages.
func (d *Document) PageBreak() {
	if d.body.Len() != 0 {
		d.bodyParts = append(d.bodyParts, d.body.String())
		d.body.Reset()
	}
}

// Paragraph appends escaped text, preserving line breaks.
func (d *Document) Paragraph(text string) {
	d.paragraph("p", "", text)
}

// ParagraphWithNext asks the renderer to keep the paragraph with the next block.
func (d *Document) ParagraphWithNext(text string) {
	d.paragraph("p", "lead", text)
}

// ParagraphRuns appends a paragraph made of individually colored text runs.
func (d *Document) ParagraphRuns(runs ...document.ParagraphRun) {
	d.openParagraph("p", "")
	for _, run := range runs {
		d.run(run.Text, run.TextColor, false)
	}
	d.body.WriteString("</p>")
}

func (d *Document) openParagraph(tag, class string) {
	d.body.WriteString("<" + tag)
	if class != "" {
		d.body.WriteString(` class="` + class + `"`)
	}
	if len(d.bookmarks) == 0 {
		d.body.WriteString(">")
		return
	}
	d.body.WriteString(` id="` + anchor(d.bookmarks[0]) + `">`)
	for _, name := range d.bookmarks[1:] {
		d.body.WriteString(`<span id="` + anchor(name) + `"></span>`)
	}
	d.bookmarks = d.bookmarks[:0]
}

func (d *Document) paragraph(tag, class, text string) {
	d.openParagraph(tag, class)
	d.body.WriteString(textHTML(text))
	d.body.WriteString("</" + tag + ">")
}

// CodeTable appends a headed block of escaped monospaced text.
func (d *Document) CodeTable(header, code string) {
	fmt.Fprintf(&d.body, "<table><thead><tr><th>%s</th></tr></thead><tbody><tr><td><pre>%s</pre></td></tr></tbody></table>",
		textHTML(header), textHTML(code))
}

// AddTable renders the shared Word table data using its column widths in
// twips. Cell text, colors, emphasis, tones and internal links are portable.
// Invalid table dimensions produce an error when the source is written.
func (d *Document) AddTable(table document.Table) {
	if d.err != nil {
		return
	}
	if len(table.Headers) == 0 || len(table.Widths) != len(table.Headers) {
		d.err = fmt.Errorf("PDF table requires one width per column")
		return
	}
	var width float64
	for _, column := range table.Widths {
		if column <= 0 {
			d.err = fmt.Errorf("PDF table widths must be positive")
			return
		}
		width += float64(column)
	}
	fmt.Fprintf(&d.body, `<table style="width:%.2fpt`, width/20)
	if table.FontSize != 0 {
		fmt.Fprintf(&d.body, ";font-size:%.1fpt", float64(table.FontSize)/2)
	}
	d.body.WriteString(`"><thead><tr>`)
	// The renderer resolves fixed-layout column hints from the first row.
	for index, header := range table.Headers {
		fmt.Fprintf(&d.body, `<th style="width:%.4f%%">%s</th>`,
			100*float64(table.Widths[index])/width, textHTML(header))
	}
	d.body.WriteString("</tr></thead><tbody>")
	for _, row := range table.Rows {
		if len(row) != len(table.Headers) {
			d.err = fmt.Errorf("PDF table row does not match its header")
			return
		}
		d.body.WriteString("<tr>")
		for _, cell := range row {
			d.cell(cell)
		}
		d.body.WriteString("</tr>")
	}
	d.body.WriteString("</tbody></table>")
}

func (d *Document) cell(cell document.Cell) {
	d.body.WriteString("<td")
	if cell.Tone == document.Difference || cell.Tone == document.Unavailable {
		d.body.WriteString(` class="` + string(cell.Tone) + `"`)
	}
	if cell.Bookmark != "" {
		d.body.WriteString(` id="` + anchor(cell.Bookmark) + `"`)
	}
	switch cell.Alignment {
	case "left", "center", "right", "justify":
		d.body.WriteString(` style="text-align:` + cell.Alignment + `"`)
	}
	d.body.WriteString(">")
	link := cell.LinkTarget
	if link != "" {
		d.body.WriteString(`<a href="#` + anchor(link) + `">`)
	} else if cell.ScreenTip != "" && (strings.TrimSpace(cell.Text) != "" || len(cell.Runs) > 0) {
		link = d.notes.Add(cell.ScreenTip)
		d.body.WriteString(`<a class="pdf-note" href="` + link + `">`)
	}
	if cell.Bold {
		d.body.WriteString("<strong>")
	}
	if len(cell.Runs) == 0 {
		d.run(cell.Text, cell.TextColor, false)
	}
	for _, run := range cell.Runs {
		color := run.TextColor
		if color == "" {
			color = cell.TextColor
		}
		d.run(run.Text, color, run.Italic)
	}
	if cell.Bold {
		d.body.WriteString("</strong>")
	}
	if link != "" {
		d.body.WriteString("</a>")
	}
	d.body.WriteString("</td>")
}

func (d *Document) run(text, color string, italic bool) {
	if italic {
		d.body.WriteString("<em>")
	}
	colorOK := len(color) == 6 && strings.IndexFunc(color, func(r rune) bool {
		return !strings.ContainsRune("0123456789abcdefABCDEF", r)
	}) == -1
	if colorOK {
		d.body.WriteString(`<span style="color:#` + color + `">`)
	}
	d.body.WriteString(textHTML(text))
	if colorOK {
		d.body.WriteString("</span>")
	}
	if italic {
		d.body.WriteString("</em>")
	}
}
