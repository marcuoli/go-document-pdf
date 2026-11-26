package reportfonts

import (
	"archive/zip"
	"context"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	wordNS           = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	relationNS       = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	strictWordNS     = "http://purl.oclc.org/ooxml/wordprocessingml/main"
	strictRelationNS = "http://purl.oclc.org/ooxml/officeDocument/relationships"
	maxXML           = 2 << 20
	maxFace          = 32 << 20
)

type relationship struct {
	ID         string `xml:"Id,attr"`
	Type       string `xml:"Type,attr"`
	Target     string `xml:"Target,attr"`
	TargetMode string `xml:"TargetMode,attr"`
}

type relationships struct {
	Entries []relationship `xml:"Relationship"`
}

type fontTable struct {
	XMLName xml.Name
	Fonts   []struct {
		Attrs   []xml.Attr `xml:",any,attr"`
		Regular []struct {
			Attrs []xml.Attr `xml:",any,attr"`
		} `xml:"embedRegular"`
	} `xml:"font"`
}

// Stage reads only regular Segoe faces referenced by the document font table.
// The destination is a private export directory owned by the caller.
func Stage(ctx context.Context, docx *zip.Reader, directory string) (Faces, error) {
	parts := make(map[string]*zip.File, len(docx.File))
	for _, entry := range docx.File {
		if _, exists := parts[entry.Name]; exists {
			return Faces{}, fmt.Errorf("duplicate DOCX part %q", entry.Name)
		}
		parts[entry.Name] = entry
	}
	const document = "word/document.xml"
	var documentRels relationships
	if err := readXML(parts[relsPath(document)], &documentRels); err != nil {
		return Faces{}, err
	}
	var tablePath string
	for _, rel := range documentRels.Entries {
		if !relationType(rel.Type, "fontTable") {
			continue
		}
		if tablePath != "" {
			return Faces{}, fmt.Errorf("DOCX has multiple font tables")
		}
		name, err := internalTarget(document, rel)
		if err != nil {
			return Faces{}, err
		}
		tablePath = name
	}
	if tablePath == "" {
		return Faces{}, nil
	}
	if parts[tablePath] == nil {
		return Faces{}, fmt.Errorf("DOCX font table is missing")
	}
	var table fontTable
	if err := readXML(parts[tablePath], &table); err != nil {
		return Faces{}, err
	}
	ns := table.XMLName.Space
	if table.XMLName.Local != "fonts" || (ns != wordNS && ns != strictWordNS) {
		return Faces{}, fmt.Errorf("invalid DOCX font table")
	}
	var result Faces
	var fontRels relationships
	for _, font := range table.Fonts {
		name := attr(font.Attrs, ns, "name")
		var available *bool
		switch {
		case strings.EqualFold(name, Symbol):
			name, available = Symbol, &result.Symbol
		case strings.EqualFold(name, Emoji):
			name, available = Emoji, &result.Emoji
		default:
			continue
		}
		if len(font.Regular) == 0 {
			continue
		}
		if len(font.Regular) != 1 || *available {
			return Faces{}, fmt.Errorf("duplicate embedded %s face", name)
		}
		if err := ctx.Err(); err != nil {
			return Faces{}, err
		}
		// Font relationships are irrelevant for a table without either face.
		if !result.Symbol && !result.Emoji {
			if err := readXML(parts[relsPath(tablePath)], &fontRels); err != nil {
				return Faces{}, err
			}
		}
		attrs := font.Regular[0].Attrs
		idNS := relationNS
		if ns == strictWordNS {
			idNS = strictRelationNS
		}
		face, err := embeddedFace(parts, tablePath, fontRels, attr(attrs, idNS, "id"), attr(attrs, ns, "fontKey"))
		if err != nil {
			return Faces{}, fmt.Errorf("load embedded %s: %w", name, err)
		}
		if err := os.WriteFile(filepath.Join(directory, name+".ttf"), face, 0600); err != nil {
			return Faces{}, err
		}
		*available = true
	}
	if result.Symbol || result.Emoji {
		result.Directory = directory
	}
	return result, ctx.Err()
}

func relsPath(part string) string {
	return path.Join(path.Dir(part), "_rels", path.Base(part)+".rels")
}

func relationType(value, kind string) bool {
	return value == relationNS+"/"+kind || value == strictRelationNS+"/"+kind
}

func attr(attrs []xml.Attr, space, name string) string {
	for _, a := range attrs {
		if a.Name.Space == space && a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func internalTarget(owner string, rel relationship) (string, error) {
	target := rel.Target
	if rel.TargetMode != "" && rel.TargetMode != "Internal" {
		return "", fmt.Errorf("external DOCX font relationship")
	}
	if target == "" || strings.ContainsAny(target, "\\:#?%") || strings.HasPrefix(target, "//") {
		return "", fmt.Errorf("invalid DOCX font target")
	}
	if strings.HasPrefix(target, "/") {
		target = path.Clean(strings.TrimPrefix(target, "/"))
	} else {
		target = path.Join(path.Dir(owner), target)
	}
	if target == "." || target == ".." || strings.HasPrefix(target, "../") {
		return "", fmt.Errorf("DOCX font target escapes the package")
	}
	return target, nil
}

func readXML(entry *zip.File, output any) error {
	if entry == nil {
		return nil
	}
	data, err := readPart(entry, maxXML)
	if err != nil {
		return err
	}
	return xml.Unmarshal(data, output)
}

func readPart(entry *zip.File, limit int64) ([]byte, error) {
	if entry == nil || !entry.Mode().IsRegular() || entry.UncompressedSize64 > uint64(limit) {
		return nil, fmt.Errorf("missing, invalid or oversized DOCX font part")
	}
	r, err := entry.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("oversized DOCX font part")
	}
	return data, nil
}

func embeddedFace(parts map[string]*zip.File, owner string, rels relationships, id, key string) ([]byte, error) {
	var part string
	for _, rel := range rels.Entries {
		if id == "" || rel.ID != id {
			continue
		}
		if part != "" || !relationType(rel.Type, "font") {
			return nil, fmt.Errorf("invalid DOCX font relationship")
		}
		target, err := internalTarget(owner, rel)
		if err != nil {
			return nil, err
		}
		part = target
	}
	data, err := readPart(parts[part], maxFace)
	if err != nil {
		return nil, err
	}
	key = strings.NewReplacer("{", "", "}", "", "-", "").Replace(key)
	guid, err := hex.DecodeString(key)
	if err != nil || len(guid) != 16 || len(data) < 32 {
		return nil, fmt.Errorf("invalid obfuscated DOCX font")
	}
	// ECMA-376 repeats the reversed GUID across the first 32 font bytes.
	for index := range data[:32] {
		data[index] ^= guid[15-index%16]
	}
	if string(data[:4]) != "\x00\x01\x00\x00" {
		return nil, fmt.Errorf("embedded DOCX font is not a TrueType outline")
	}
	return data, nil
}
