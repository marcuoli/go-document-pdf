// Package htmlpdf renders inline HTML as PDF without a browser or an Office installation.
package htmlpdf

import (
	"context"
	"encoding/base64"
	"io"
	"slices"

	engine "github.com/chinmay-sawant/gowkhtmltopdf"
	"github.com/marcuoli/go-document-docx/publication"
)

// Options controls the document title, optional linked table of contents and
// fonts. Use licensed TrueType fonts covering the report's characters for
// consistent output across platforms. An empty ContentsTitle omits the TOC.
type Options struct {
	Title         string
	ContentsTitle string
	FontPaths     []string
}

// New returns a replayable PDF source holding its own copy of the inline HTML
// and options. Filesystem and network subresource reads are disabled. Embed
// report graphics and styles in the HTML. FontPaths explicitly admits local fonts.
func New(html []byte, options Options) publication.Source {
	d := newLayout([]Page{{HTML: html}}, Layout{Title: options.Title, FontPaths: options.FontPaths})
	if options.ContentsTitle != "" {
		d.TOC = &engine.TOC{Caption: options.ContentsTitle, ForwardLinks: new(true)}
	}
	return source{document: d}
}

// Page supplies one inline HTML document, which may flow across several A4
// pages. Set ExcludeFromOutline for navigation pages such as a custom contents.
type Page struct {
	HTML               []byte
	ExcludeFromOutline bool
}

// Layout sets the shared A4 margins and optional inline cover and running bands.
// MarginMM of zero uses the renderer default. Running bands can use [page]
// and [topage]. The cover is emitted first and has no running bands.
type Layout struct {
	Title           string
	FontPaths       []string
	CoverHTML       []byte
	HeaderHTML      []byte
	FooterHTML      []byte
	MarginMM        float64
	FooterSpacingMM float64
}

// NewLayout returns a replayable source owning the supplied HTML and settings.
// Write checks the layout before rendering. Embed resources with data URIs.
// Filesystem and network subresource reads are restricted as with New.
func NewLayout(pages []Page, layout Layout) publication.Source {
	return source{document: newLayout(pages, layout)}
}

func newLayout(pages []Page, layout Layout) engine.Document {
	margin := layout.MarginMM
	d := engine.Document{
		PageSize:  "A4",
		Title:     layout.Title,
		Outline:   new(true),
		Margin:    engine.Margin{Top: margin, Right: margin, Bottom: margin, Left: margin},
		FontPaths: slices.Clone(layout.FontPaths),
		// Inline data resources are resolved separately from network and
		// filesystem resources.
		Network: &engine.NetworkPolicy{},
		Pages:   make([]engine.Page, 0, len(pages)),
	}
	if len(layout.CoverHTML) != 0 {
		d.Cover = &engine.Page{Source: engine.HTML(layout.CoverHTML)}
	}
	if len(layout.HeaderHTML) != 0 {
		d.Header = &engine.HeaderFooter{HTMLURL: htmlData(layout.HeaderHTML)}
	}
	if len(layout.FooterHTML) != 0 {
		d.Footer = &engine.HeaderFooter{HTMLURL: htmlData(layout.FooterHTML), Spacing: layout.FooterSpacingMM}
	}
	for _, page := range pages {
		content := engine.HTML(page.HTML)
		if len(page.HTML) != 0 && (d.Header != nil || d.Footer != nil) {
			// v0.2.6 reuses an object's InlineHTML when it loads a band.
			// The engine's data-document loader leaves InlineHTML unset.
			content = engine.File(htmlData(page.HTML))
		}
		d.Pages = append(d.Pages, engine.Page{Source: content, IncludeInOutline: new(!page.ExcludeFromOutline)})
	}
	return d
}

func htmlData(html []byte) string {
	return "data:text/html;base64," + base64.StdEncoding.EncodeToString(html)
}

type source struct {
	document engine.Document
}

func (s source) Write(ctx context.Context, w io.Writer) error {
	return s.document.WritePDF(ctx, w)
}
