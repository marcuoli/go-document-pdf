package report

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"image/png"
	"io"
	"math"
	"strings"

	"github.com/marcuoli/go-document-docx/publication"
	"github.com/marcuoli/go-document-pdf/htmlpdf"
)

const portableStyles = `body { margin: 0; font-family: "Times New Roman", serif; font-size: 9.5pt; color: #151515; }
h1, h2, h3 { page-break-after: avoid; margin: 10pt 0 4pt; }
h1 { font-size: 15pt; } h2 { font-size: 11pt; } h3 { font-size: 10pt; }
p { margin: 3pt 0 5pt; white-space: pre-wrap; } p.lead { page-break-after: avoid; }
table { width: 100%; table-layout: fixed; border-collapse: collapse; margin-bottom: 8pt; font-size: 8pt; }
th, td { border: 0.5pt solid #000; padding: 1pt 2.5pt; vertical-align: middle; white-space: pre-wrap; overflow-wrap: break-word; }
th { color: #fff; background-color: #000; text-align: left; }
thead { display: table-header-group; }
a { color: #235865; }
td.difference { background-color: #fff0d6; } td.unavailable { background-color: #f2f2f2; }
pre { margin: 0; white-space: pre-wrap; overflow-wrap: break-word; font-size: 8pt; }`

const portableCoverStyles = `@page { margin: 0; }
body { margin: 0; font-family: "Times New Roman", serif; }
.cover { position: relative; width: 210mm; height: 297mm; background: #f8f5ee; }
.cover-band { position: absolute; top: 0; left: 0; } .cover-band img { display: block; width: 6.7mm; height: 297mm; }
.cover-logo { position: absolute; top: 19mm; left: 19mm; } .cover-logo img { display: block; width: 41mm; }
.cover-copy { position: absolute; top: 121mm; left: 19mm; width: 170mm; overflow-wrap: break-word; }
.cover-heading { min-height: 69mm; } .cover-heading p:first-child { width: 131mm; }
.cover-heading p { font-size: 31pt; line-height: 1.12; margin: 0 0 6mm; }
.cover-meta { font-size: 10pt; }
.cover-meta p { margin: 0 0 1mm; } .cover-details { margin-bottom: 6mm; } .cover-details p { margin: 0; }
.cover-rule { width: 8mm; height: 1mm; background: #ffc759; margin-bottom: 9mm; }`

const portableFooterStyles = `body { margin: 0; font: 7.5pt "Times New Roman", serif; }
table { width: 100%; table-layout: fixed; border-collapse: collapse; }
td { padding: 0; vertical-align: top; }
.page { width: 5mm; } .logo { width: 21mm; text-align: right; }
img { width: 19mm; } p { margin: 0; } .copyright { font-size: 6.5pt; margin-top: 2pt; }`

// Options controls the text and artwork for an A4 report. MarginTwips applies
// to the body and running page bands. Use a 19.05 mm margin by leaving it
// zero. ContentsTitle enables a linked list of headings after the cover.
// Opening precedes the contents on its own page and its headings are linked
// from the contents. Introduction flows with the first body block. Closing
// starts on a new page after the body without adding contents entries.
type Options struct {
	Title, ContentsTitle, Introduction string
	MarginTwips                        int
	Cover                              *Cover
	HeaderImagePNG                     []byte
	Footer                             *Footer
	Opening, Closing                   []Section
}

// Cover supplies the subtitle and two groups of metadata shown below the cover
// heading. LogoPNG and TrimPNG supply optional upper-left logo and trim artwork.
// CenteredLogo adds an independent mark centered on the physical cover.
type Cover struct {
	Subtitle          string
	Details, Metadata []string
	LogoPNG, TrimPNG  []byte
	LogoAlt           string
	CenteredLogo      *CoverLogo
}

// CoverLogo places a second PNG in a caller-sized box. TopMM is measured
// from the top of the A4 page. The image keeps its aspect ratio and fits
// within MaxWidthMM and MaxHeightMM. Alt describes the image. The PNG may
// contain up to 32 MiB and 16 Mi pixels, with a limit of 16,384 per side.
// AspectWidth and AspectHeight can supply the original pixel dimensions
// for layout after resampling has rounded the PNG dimensions. Both zero
// uses the PNG aspect ratio. Otherwise both dimensions must be positive.
type CoverLogo struct {
	PNG                       []byte
	Alt                       string
	TopMM                     float64
	MaxWidthMM, MaxHeightMM   float64
	AspectWidth, AspectHeight int
}

// Footer supplies the label and two lines of text for numbered pages.
// LogoPNG is optional. Label is shown in bold before Text.
type Footer struct {
	Label, Text, Secondary string
	LogoPNG                []byte
	LogoAlt                string
}

// Source snapshots the current document and page options. It can be written
// more than once. All report text is escaped and its images are embedded.
func (d *Document) Source(options Options) publication.Source {
	if d.err != nil {
		return failedSource{d.err}
	}
	margin := options.MarginTwips
	if margin == 0 {
		margin = 1080
	}
	layout := htmlpdf.Layout{Title: options.Title, MarginMM: float64(margin) * 25.4 / 1440, FooterSpacingMM: 10}
	if options.Cover != nil {
		cover, err := coverHTML(options.Title, *options.Cover)
		if err != nil {
			return failedSource{err}
		}
		layout.CoverHTML = cover
	}
	if len(options.HeaderImagePNG) != 0 {
		headerCSS := fmt.Sprintf(`body {margin: 0;} img {display: block; width: 210mm; height: 5.6mm; margin-left: -%.4fmm;}`, layout.MarginMM)
		layout.HeaderHTML = htmlDocument(headerCSS, imageHTML(options.HeaderImagePNG, ""))
	}
	if options.Footer != nil {
		layout.FooterHTML = footerHTML(*options.Footer)
	}
	pages := make([]htmlpdf.Page, 0, 4+len(d.bodyParts))
	opening, openingHeadings := sectionHTML(options.Opening, "portable-opening")
	if opening != "" {
		pages = append(pages, htmlpdf.Page{HTML: htmlDocument(portableStyles, opening)})
	}
	if options.ContentsTitle != "" && len(openingHeadings)+len(d.headings) > 0 {
		markup := d.contents(options.ContentsTitle, openingHeadings)
		pages = append(pages, htmlpdf.Page{HTML: htmlDocument(portableStyles, markup), ExcludeFromOutline: true})
	}
	var introduction string
	if options.Introduction != "" {
		introduction = "<p>" + textHTML(options.Introduction) + "</p>"
	}
	for _, part := range d.bodyParts {
		pages = append(pages, htmlpdf.Page{HTML: htmlDocument(portableStyles, introduction+part)})
		introduction = ""
	}
	if last := introduction + d.body.String(); last != "" || len(pages) == 0 {
		pages = append(pages, htmlpdf.Page{HTML: htmlDocument(portableStyles, last)})
	}
	if closing, _ := sectionHTML(options.Closing, ""); closing != "" {
		pages = append(pages, htmlpdf.Page{HTML: htmlDocument(portableStyles, closing), ExcludeFromOutline: true})
	}
	return portableSource{pages: pages, layout: layout}
}

func (d *Document) contents(title string, opening []heading) string {
	var contents strings.Builder
	fmt.Fprintf(&contents, "<h1>%s</h1>", textHTML(title))
	for _, headings := range [][]heading{opening, d.headings} {
		for _, h := range headings {
			fmt.Fprintf(&contents, `<p style="margin-left:%dpt"><a href="#%s">%s</a></p>`,
				(h.level-1)*11, anchor(h.id), textHTML(h.text))
		}
	}
	return contents.String()
}

func coverHTML(title string, cover Cover) ([]byte, error) {
	centered, err := centeredCoverLogoHTML(cover.CenteredLogo)
	if err != nil {
		return nil, err
	}
	markup := fmt.Sprintf(`<div class="cover">
<div class="cover-band">%s</div><div class="cover-logo">%s</div>%s
<div class="cover-copy"><div class="cover-heading"><p>%s</p><p>%s</p></div>
<div class="cover-meta"><div class="cover-rule"></div><div class="cover-details">%s</div>%s</div></div></div>`,
		imageHTML(cover.TrimPNG, ""), imageHTML(cover.LogoPNG, cover.LogoAlt), centered,
		textHTML(title), textHTML(cover.Subtitle), paragraphs(cover.Details), paragraphs(cover.Metadata))
	return htmlDocument(portableCoverStyles, markup), nil
}

func centeredCoverLogoHTML(logo *CoverLogo) (string, error) {
	if logo == nil {
		return "", nil
	}
	const widthMM, heightMM = 210, 297
	if !(logo.TopMM >= 0 && logo.TopMM < heightMM &&
		logo.MaxWidthMM > 0 && logo.MaxWidthMM <= widthMM &&
		logo.MaxHeightMM > 0 && logo.MaxHeightMM <= heightMM) {
		return "", errors.New("centered cover logo has invalid A4 dimensions")
	}
	if (logo.AspectWidth != 0 || logo.AspectHeight != 0) && (logo.AspectWidth <= 0 || logo.AspectHeight <= 0) {
		return "", errors.New("centered cover logo requires two positive original aspect dimensions")
	}
	// Match the renderer's image bounds before allocating decoded pixels.
	const maxPNGBytes, maxPNGSide, maxPNGPixels = 32 << 20, 16_384, 16 << 20
	if len(logo.PNG) > maxPNGBytes {
		return "", errors.New("centered cover logo exceeds the PNG byte limit")
	}
	config, err := png.DecodeConfig(bytes.NewReader(logo.PNG))
	if err != nil {
		return "", fmt.Errorf("read centered cover logo: %w", err)
	}
	if config.Width > maxPNGSide || config.Height > maxPNGSide || int64(config.Width)*int64(config.Height) > maxPNGPixels {
		return "", errors.New("centered cover logo exceeds the PNG pixel limit")
	}
	if _, err := png.Decode(bytes.NewReader(logo.PNG)); err != nil {
		return "", fmt.Errorf("decode centered cover logo: %w", err)
	}
	aspectWidth, aspectHeight := config.Width, config.Height
	if logo.AspectWidth != 0 {
		aspectWidth, aspectHeight = logo.AspectWidth, logo.AspectHeight
	}
	w, h := float64(aspectWidth), float64(aspectHeight)
	scale := math.Min(logo.MaxWidthMM/w, logo.MaxHeightMM/h)
	width, height := w*scale, h*scale
	if height > heightMM-logo.TopMM {
		return "", errors.New("centered cover logo extends below the A4 cover")
	}
	// The renderer places absolute offsets on the wrapper and sizes the PNG independently.
	return fmt.Sprintf(`<div class="cover-centered-logo" style="position:absolute;left:%.6fmm;top:%.6fmm"><img style="display:block;width:%.6fmm;height:%.6fmm" src="data:image/png;base64,%s" alt="%s"></div>`,
		(widthMM-width)/2, logo.TopMM, width, height, base64.StdEncoding.EncodeToString(logo.PNG), html.EscapeString(logo.Alt)), nil
}

func footerHTML(footer Footer) []byte {
	firstLine := textHTML(footer.Text)
	if footer.Label != "" {
		firstLine = "<strong>" + textHTML(footer.Label) + "</strong> / " + firstLine
	}
	return htmlDocument(portableFooterStyles,
		fmt.Sprintf(`<table><tr><td class="page"><strong>[page]</strong></td><td>
<p>%s</p><p class="copyright">%s</p>
</td><td class="logo">%s</td></tr></table>`,
			firstLine, textHTML(footer.Secondary), imageHTML(footer.LogoPNG, footer.LogoAlt)))
}

func paragraphs(lines []string) string {
	var rendered strings.Builder
	for _, line := range lines {
		rendered.WriteString("<p>" + textHTML(line) + "</p>")
	}
	return rendered.String()
}

func imageHTML(png []byte, alt string) string {
	if len(png) == 0 {
		return ""
	}
	return `<img src="data:image/png;base64,` + base64.StdEncoding.EncodeToString(png) + `" alt="` + html.EscapeString(alt) + `">`
}

func htmlDocument(styles, body string) []byte {
	return []byte("<!DOCTYPE html><html><head><meta charset=\"utf-8\"><style>" + styles + "</style></head><body>" + body + "</body></html>")
}

type failedSource struct{ err error }

func (s failedSource) Write(ctx context.Context, _ io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.err
}
