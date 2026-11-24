// Package htmlpdf renders inline HTML as PDF without a browser or an Office installation.
package htmlpdf

import (
	"context"
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
	options.FontPaths = slices.Clone(options.FontPaths)
	return source{html: slices.Clone(html), options: options}
}

type source struct {
	html    []byte
	options Options
}

func (s source) Write(ctx context.Context, w io.Writer) error {
	outline := true
	document := engine.Document{
		Pages:     []engine.Page{{Source: engine.Content{HTML: s.html}}},
		PageSize:  "A4",
		Title:     s.options.Title,
		Outline:   &outline,
		FontPaths: s.options.FontPaths,
		// An explicit empty policy denies all network schemes. Inline data
		// resources are resolved separately without network or filesystem access.
		Network: &engine.NetworkPolicy{},
	}
	if s.options.ContentsTitle != "" {
		document.TOC = &engine.TOC{Caption: s.options.ContentsTitle, ForwardLinks: &outline}
	}
	return document.WritePDF(ctx, w)
}
