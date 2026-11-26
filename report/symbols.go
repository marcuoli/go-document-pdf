package report

import (
	"context"
	"io"
	"slices"
	"strings"

	"github.com/marcuoli/go-document-pdf/htmlpdf"
	"github.com/marcuoli/go-document-pdf/internal/notes"
	"github.com/marcuoli/go-document-pdf/internal/reportfonts"
)

const (
	symbolSpan = "<span data-pdf-mark=\"symbol\">"
	emojiSpan  = "<span data-pdf-mark=\"warning\">"
	symbolFont = "<span style=\"font-family:'" + reportfonts.Symbol + "'\">"
	emojiFont  = "<span style=\"font-family:'" + reportfonts.Emoji + "'\">"
)

var symbolMarks = func() string {
	var marks strings.Builder
	for arrow := '\u2190'; arrow <= '\u21ff'; arrow++ {
		marks.WriteRune(arrow)
	}
	marks.WriteString("✓✔✕✖✗✘")
	return marks.String()
}()

var reportMarks = func() *strings.Replacer {
	var args []string
	for _, symbol := range symbolMarks {
		text := string(symbol)
		args = append(args, text, symbolSpan+text+"</span>")
	}
	args = append(args, "⚠️", emojiSpan+"⚠️</span>", "⚠", emojiSpan+"⚠</span>")
	return strings.NewReplacer(args...)
}()

type portableSource struct {
	pages  []htmlpdf.Page
	layout htmlpdf.Layout
	notes  notes.Set
}

func (s portableSource) Write(ctx context.Context, w io.Writer) error {
	return reportfonts.Render(ctx, func(faces reportfonts.Faces) error {
		layout := s.layout
		pages := slices.Clone(s.pages)
		replace := markStyles(faces)
		style := func(html []byte) []byte {
			return []byte(replace.Replace(string(html)))
		}
		layout.CoverHTML = style(layout.CoverHTML)
		layout.HeaderHTML = style(layout.HeaderHTML)
		layout.FooterHTML = style(layout.FooterHTML)
		if faces.Directory != "" {
			layout.FontPaths = []string{faces.Directory}
		}
		for index := range pages {
			pages[index].HTML = style(pages[index].HTML)
		}
		return s.notes.Render(ctx, w, htmlpdf.NewLayout(pages, layout), faces.Directory)
	})
}

func markStyles(faces reportfonts.Faces) *strings.Replacer {
	var args []string
	for _, symbol := range symbolMarks {
		text := string(symbol)
		styled := text
		if faces.Symbol {
			styled = symbolFont + text + "</span>"
		}
		args = append(args, symbolSpan+text+"</span>", styled)
	}
	for _, warning := range []string{"⚠️", "⚠"} {
		styled := warning
		if faces.Emoji {
			// The PDF paints the embedded TrueType outline of the base glyph.
			// Its Unicode mapping must not acquire a separate selector glyph.
			styled = emojiFont + "⚠</span>"
		}
		args = append(args, emojiSpan+warning+"</span>", styled)
	}
	return strings.NewReplacer(args...)
}
