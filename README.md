# go-document-pdf

Publish a PDF from a native Windows Word export source or a portable Go renderer.
The caller supplies content and artwork. The package selects the renderer. PDF publication is independent of
DOCX field refresh.

```go
source := htmlpdf.New([]byte("<h1>Report</h1><p>Assessment results</p>"),
    htmlpdf.Options{Title: "Report", ContentsTitle: "Contents"})
receipt, err := pdf.Save(ctx, "report.pdf", false, pdf.Sources{
    PortablePDF: source,
})
```

Import `github.com/marcuoli/go-document-pdf` as `pdf` and
`github.com/marcuoli/go-document-pdf/htmlpdf` as `htmlpdf`.

On Windows a caller can also supply its Word export source in `WordPDF`.
The package selects that source when available. The portable source is used
on other platforms and when Word reports that its automation is unavailable
before anything is written. A failed render preserves an existing PDF.

The portable renderer uses inline HTML and generates its own pagination, bookmark
destinations and TOC. Use licensed fonts covering the characters in your report.
The package uses gowkhtmltopdf for HTML layout and the shared document publication
transaction for installation. The Word adapter is supplied by the caller so the
existing DOCX Word SDK can own all Office automation.

## Structured reports

Build a portable document with the same table and text-run types used by
go-document-docx. The PDF package escapes the text, applies the selected
column widths and renders links, the cover and running page bands.

```go
var body report.Document
body.BookmarkNext("Summary")
body.Heading("Summary", 1)
body.AddTable(document.Table{
    Headers: []string{"Property", "Value"},
    Widths:  []int{3600, 6000}, // Twips, matching the Word table model.
    Rows:    [][]document.Cell{{{Text: "Region"}, {Text: "West"}}},
})
portable := body.Source(report.Options{
    Title:         "Assessment",
    ContentsTitle: "Contents",
    Cover:         &report.Cover{Subtitle: "Quarterly review"},
    Footer:        &report.Footer{Label: "Technical Brief", Text: "Assessment"},
})
receipt, err := pdf.SaveFromDOCX(ctx, pdfPath, docxPath, docxReceipt, portable, false)
```

Import `github.com/marcuoli/go-document-pdf/report` as `report` and
`github.com/marcuoli/go-document-docx/document` as `document`.
Supply caller-owned PNGs in the cover, header and footer options when needed.
The structured contents follows the cover and any opening sections, and links
to the opening and report headings. Use `Options.Opening` for introductory
sections, `Document.PageBreak` to start a new body page, and `Options.Closing`
for a separate closing page. Sections support paragraphs and external HTTP,
HTTPS and mail links.
`SaveFromDOCX` verifies the published DOCX bytes staged for Word. The Office
automation continues to use the shared Word adapter. PDFs can be published
independently of any DOCX field-refresh operation. The structured portable
source uses Segoe UI Symbol for arrows, checks and crosses, and Segoe UI
Emoji for warnings, when the matching regular faces are embedded in the
published DOCX. The portable PDF uses the warning glyph's outline and the
report's text color. Missing faces use the renderer's default fonts.

PDFs omit automatic status ScreenTips. Word exports a private copy with those
generated links removed, so the original DOCX keeps its ScreenTips. The PDF
keeps its visible status symbols and authored navigation destinations.
