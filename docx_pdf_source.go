package pdf

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/beevik/etree"
)

const (
	documentPart   = "word/document.xml"
	wordNS         = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	maxDocumentXML = 128 << 20
)

type docxWithoutScreenTips struct {
	document  finalizedDOCX
	directory string
}

// Word exports a private copy with the automatic status hyperlinks unwrapped.
// Doing this in XML avoids a per-link Word automation call for each cell.
func (s docxWithoutScreenTips) Write(ctx context.Context, w io.Writer) (returnedErr error) {
	input, err := os.CreateTemp(s.directory, ".pdf-docx-*")
	if err != nil {
		return err
	}
	defer func() { returnedErr = errors.Join(returnedErr, input.Close(), os.Remove(input.Name())) }()
	if err := s.document.Write(ctx, input); err != nil {
		return err
	}
	archive, err := zip.NewReader(input, s.document.receipt.Size)
	if err != nil {
		return err
	}
	body, err := stripGeneratedScreenTips(ctx, archive)
	if err != nil {
		return err
	}
	if body == nil {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err := io.Copy(w, io.NewSectionReader(input, 0, s.document.receipt.Size))
		return errors.Join(err, ctx.Err())
	}
	return writePDFStage(ctx, w, archive, body)
}

func stripGeneratedScreenTips(ctx context.Context, archive *zip.Reader) ([]byte, error) {
	main, err := readScreenTipDocument(archive)
	if err != nil {
		return nil, err
	}
	changed := false
	for _, link := range main.FindElements(".//hyperlink") {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !isGeneratedScreenTip(link) {
			continue
		}
		parent, index := link.Parent(), link.Index()
		for len(link.Child) != 0 {
			parent.InsertChildAt(index, link.RemoveChildAt(0))
			index++
		}
		parent.RemoveChild(link)
		changed = true
	}
	if !changed {
		return nil, ctx.Err()
	}
	return main.WriteToBytes()
}

// The paired bookmarks distinguish an automatic cell tip from navigation
// authored with an ordinary internal link or a similar-looking bookmark name.
func isGeneratedScreenTip(link *etree.Element) bool {
	if !wordElement(link, "hyperlink") || link.SelectAttr("id") != nil {
		return false
	}
	tip := link.SelectAttrValue(link.Space+":tooltip", "")
	target := link.SelectAttrValue(link.Space+":anchor", "")
	if tip == "" || !numberedBookmark(target, "_DocxCellTip") {
		return false
	}
	marker := link.PrevSibling()
	if !wordElement(marker, "bookmarkStart") || !numberedBookmark(marker.SelectAttrValue("name", ""), "_DocxCellTipMark") {
		return false
	}
	destination := marker.PrevSibling()
	if !wordElement(destination, "bookmarkStart") || destination.SelectAttrValue("name", "") != target {
		return false
	}
	endMarker := link.NextSibling()
	if !matchingBookmarkEnd(marker, endMarker) || !matchingBookmarkEnd(destination, endMarker.NextSibling()) {
		return false
	}
	for parent := link.Parent(); parent != nil; parent = parent.Parent() {
		if wordElement(parent, "tc") {
			return true
		}
	}
	return false
}

func wordElement(el *etree.Element, name string) bool {
	return el != nil && el.Tag == name && el.NamespaceURI() == wordNS
}

func numberedBookmark(name, prefix string) bool {
	raw, ok := strings.CutPrefix(name, prefix)
	n, err := strconv.Atoi(raw)
	return ok && err == nil && n > 0 && strconv.Itoa(n) == raw
}

func matchingBookmarkEnd(start, end *etree.Element) bool {
	id := start.SelectAttrValue("id", "")
	return id != "" && wordElement(end, "bookmarkEnd") && end.SelectAttrValue("id", "") == id
}

func readScreenTipDocument(archive *zip.Reader) (*etree.Document, error) {
	input, err := archive.Open(documentPart)
	if err != nil {
		return nil, err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > maxDocumentXML {
		return nil, fmt.Errorf("oversized DOCX XML part %s", documentPart)
	}
	doc := etree.NewDocument()
	n, err := doc.ReadFrom(io.LimitReader(input, maxDocumentXML+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", documentPart, err)
	}
	if n > maxDocumentXML || doc.Root() == nil {
		return nil, fmt.Errorf("invalid or oversized DOCX XML part %s", documentPart)
	}
	return doc, nil
}

func writePDFStage(ctx context.Context, w io.Writer, archive *zip.Reader, body []byte) (returnedErr error) {
	output := zip.NewWriter(w)
	defer func() { returnedErr = errors.Join(returnedErr, output.Close()) }()
	for _, entry := range archive.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Name != documentPart {
			if err := output.Copy(entry); err != nil {
				return err
			}
			continue
		}
		part, err := output.Create(entry.Name)
		if err != nil {
			return err
		}
		if _, err := part.Write(body); err != nil {
			return err
		}
	}
	return ctx.Err()
}
