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
	"github.com/marcuoli/go-document-docx/word"
	"github.com/marcuoli/go-document-pdf/internal/notes"
)

const (
	documentPart      = "word/document.xml"
	relationshipsPart = "word/_rels/document.xml.rels"
	wordNS            = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	linkNS            = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	packageRelNS      = "http://schemas.openxmlformats.org/package/2006/relationships"
	maxNoteXMLPart    = 128 << 20
)

type wordNoteSource struct {
	document  finalizedDOCX
	directory string
}

func (s wordNoteSource) Write(ctx context.Context, w io.Writer) (returnedErr error) {
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
	parts, popups, err := wordNoteParts(ctx, archive)
	if err != nil {
		return err
	}
	opt := word.Options{TemporaryDirectory: s.directory}
	if len(parts) == 0 {
		return word.PDF(s.document, opt).Write(ctx, w)
	}
	staged := wordNoteArchive{archive: archive, parts: parts}
	return popups.Render(ctx, w, word.PDFPreservingLinks(staged, opt), s.directory)
}

// Rewrite only the generated self-links on the private Word input. The unique
// URI carries identity across Word pagination; neither its target nor the
// visible status text needs to be guessed from the exported page.
func wordNoteParts(ctx context.Context, archive *zip.Reader) (map[string][]byte, notes.Set, error) {
	var popups notes.Set
	main, err := readNotePart(archive, documentPart)
	if err != nil {
		return nil, popups, err
	}
	var replacements []wordNoteLink
	for _, link := range main.FindElements(".//hyperlink") {
		if err := ctx.Err(); err != nil {
			return nil, popups, err
		}
		tip := generatedScreenTip(link)
		if tip == "" {
			continue
		}
		id := "rIdNote" + strconv.Itoa(len(replacements)+1)
		link.RemoveAttr(link.Space + ":anchor")
		link.RemoveAttr(link.Space + ":tooltip")
		link.CreateAttr("xmlns:noteRel", linkNS)
		link.CreateAttr("noteRel:id", id)
		replacements = append(replacements, wordNoteLink{id, popups.Add(tip)})
	}
	if len(replacements) == 0 {
		return nil, popups, nil
	}
	rels, err := readNotePart(archive, relationshipsPart)
	if errors.Is(err, os.ErrNotExist) {
		rels = etree.NewDocument()
		rels.CreateElement("Relationships").CreateAttr("xmlns", packageRelNS)
	} else if err != nil {
		return nil, popups, err
	}
	root := rels.Root()
	if root.Tag != "Relationships" || root.NamespaceURI() != packageRelNS {
		return nil, popups, errors.New("invalid DOCX relationships part")
	}
	known := make(map[string]bool)
	for entry := range root.ChildElementsSeq() {
		known[entry.SelectAttrValue("Id", "")] = true
	}
	tag := "Relationship"
	if root.Space != "" {
		tag = root.Space + ":" + tag
	}
	for _, replacement := range replacements {
		if known[replacement.id] {
			return nil, popups, errors.New("DOCX note relationship ID already exists")
		}
		entry := root.CreateElement(tag)
		entry.CreateAttr("Id", replacement.id)
		entry.CreateAttr("Type", linkNS+"/hyperlink")
		entry.CreateAttr("Target", replacement.url)
		entry.CreateAttr("TargetMode", "External")
	}
	body, err := main.WriteToBytes()
	if err != nil {
		return nil, popups, err
	}
	relationships, err := rels.WriteToBytes()
	if err != nil {
		return nil, popups, err
	}
	return map[string][]byte{documentPart: body, relationshipsPart: relationships}, popups, nil
}

type wordNoteLink struct {
	id, url string
}

// The paired bookmarks distinguish an automatic cell tip from navigation
// authored with an ordinary internal link or a similar-looking bookmark name.
func generatedScreenTip(link *etree.Element) string {
	if !wordElement(link, "hyperlink") || link.SelectAttr("id") != nil {
		return ""
	}
	tip := link.SelectAttrValue(link.Space+":tooltip", "")
	target := link.SelectAttrValue(link.Space+":anchor", "")
	if tip == "" || !numberedBookmark(target, "_DocxCellTip") {
		return ""
	}
	marker := link.PrevSibling()
	if !wordElement(marker, "bookmarkStart") || !numberedBookmark(marker.SelectAttrValue("name", ""), "_DocxCellTipMark") {
		return ""
	}
	destination := marker.PrevSibling()
	if !wordElement(destination, "bookmarkStart") || destination.SelectAttrValue("name", "") != target {
		return ""
	}
	endMarker := link.NextSibling()
	if !matchingBookmarkEnd(marker, endMarker) || !matchingBookmarkEnd(destination, endMarker.NextSibling()) {
		return ""
	}
	for parent := link.Parent(); parent != nil; parent = parent.Parent() {
		if wordElement(parent, "tc") {
			return tip
		}
	}
	return ""
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

func readNotePart(archive *zip.Reader, name string) (*etree.Document, error) {
	input, err := archive.Open(name)
	if err != nil {
		return nil, err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > maxNoteXMLPart {
		return nil, fmt.Errorf("oversized DOCX XML part %s", name)
	}
	doc := etree.NewDocument()
	n, err := doc.ReadFrom(io.LimitReader(input, maxNoteXMLPart+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	if n > maxNoteXMLPart || doc.Root() == nil {
		return nil, fmt.Errorf("invalid or oversized DOCX XML part %s", name)
	}
	return doc, nil
}

type wordNoteArchive struct {
	archive *zip.Reader
	parts   map[string][]byte
}

func (s wordNoteArchive) Write(ctx context.Context, w io.Writer) (returnedErr error) {
	output := zip.NewWriter(w)
	defer func() { returnedErr = errors.Join(returnedErr, output.Close()) }()
	missing := true
	for _, entry := range s.archive.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Name == relationshipsPart {
			missing = false
		}
		if data, ok := s.parts[entry.Name]; ok {
			if err := writeNotePart(output, entry.Name, data); err != nil {
				return err
			}
		} else if err := output.Copy(entry); err != nil {
			return err
		}
	}
	if missing {
		return writeNotePart(output, relationshipsPart, s.parts[relationshipsPart])
	}
	return ctx.Err()
}

func writeNotePart(output *zip.Writer, name string, data []byte) error {
	part, err := output.Create(name)
	if err != nil {
		return err
	}
	_, err = part.Write(data)
	return err
}
