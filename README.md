# go-document-pdf

Publish a PDF from a native Windows Word export source or a portable Go renderer.
The caller owns report content and routing. PDF publication is independent of
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
