// Package notes converts private renderer links into native PDF markup comments.
package notes

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"strconv"
	"strings"

	"github.com/marcuoli/go-document-docx/publication"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Set assigns collision-resistant link targets to literal annotation text.
type Set struct {
	prefix string
	text   map[string]string
}

// Add records text and returns its intermediate renderer link target.
func (s *Set) Add(text string) string {
	if s.text == nil {
		s.prefix = "https://pdf-note.invalid/" + strings.ToLower(rand.Text()) + "/"
		s.text = make(map[string]string)
	}
	url := s.prefix + strconv.Itoa(len(s.text)+1)
	s.text[url] = text
	return url
}

// Clone snapshots the marker assignments for a replayable rendering source.
func (s Set) Clone() Set {
	return Set{prefix: s.prefix, text: maps.Clone(s.text)}
}

// Render writes the output of source, replacing each known intermediate link
// with a nonprinting markup note at the link's final page position.
func (s Set) Render(ctx context.Context, w io.Writer, source publication.Source, temporaryDirectory string) (returnedErr error) {
	if len(s.text) == 0 {
		return source.Write(ctx, w)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	stage, err := os.CreateTemp(temporaryDirectory, ".pdf-notes-*")
	if err != nil {
		return err
	}
	defer func() { returnedErr = errors.Join(returnedErr, stage.Close(), os.Remove(stage.Name())) }()
	if err := source.Write(ctx, stage); err != nil {
		return err
	}
	if _, err := stage.Seek(0, io.SeekStart); err != nil {
		return err
	}
	conf := model.NewStatelessConfiguration()
	// Keep the graph and content streams emitted by the selected renderer.
	conf.Optimize = false
	conf.OptimizeBeforeWriting = false
	conf.WriteObjectStream = false
	conf.WriteXRefStream = false
	conf.Offline = true
	pdf, err := api.ReadContext(ctx, stage, conf)
	if err != nil {
		return fmt.Errorf("read PDF to install notes: %w", err)
	}
	if err := s.replace(ctx, pdf); err != nil {
		return err
	}
	return api.WriteContext(ctx, pdf, w)
}

func (s Set) replace(ctx context.Context, pdf *model.Context) error {
	if err := pdf.EnsurePageCount(); err != nil {
		return err
	}
	appearance, err := noteAppearance(pdf)
	if err != nil {
		return err
	}
	seen := make(map[string]placement, len(s.text))
	parents := make(map[int]struct{})
	converted := make(map[types.IndirectRef]struct{})
	for page := 1; page <= pdf.PageCount; page++ {
		dict, _, _, err := pdf.PageDict(ctx, page, false)
		if err != nil {
			return err
		}
		entries, err := pdf.DereferenceArray(dict["Annots"])
		if err != nil {
			return err
		}
		kept := entries[:0]
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			annotation, err := pdf.DereferenceDict(entry)
			if err != nil {
				return err
			}
			url, err := markerURL(pdf, annotation)
			if err != nil {
				return err
			}
			if !strings.HasPrefix(url, s.prefix) {
				kept = append(kept, entry)
				continue
			}
			text, ok := s.text[url]
			if !ok {
				return fmt.Errorf("unknown PDF note marker on page %d", page)
			}
			box, err := linkRectangle(pdf, annotation)
			if err != nil {
				return fmt.Errorf("place PDF note on page %d: %w", page, err)
			}
			prior, duplicate := seen[url]
			if duplicate {
				// Different runs of a status can create several rectangles.
				// Place one icon after the last run of the first line.
				if prior.page == page && prior.box.LL.Y < box.UR.Y && prior.box.UR.Y > box.LL.Y && box.UR.X > prior.box.UR.X {
					prior.box.UR.X = box.UR.X
					prior.annotation["Rect"] = noteRect(prior.box)
				}
				if annotation["StructParent"] == nil {
					continue
				}
			} else {
				seen[url] = placement{page, box, annotation}
			}
			replaceLink(annotation, text, box, appearance, duplicate)
			if parent := annotation.IntEntry("StructParent"); parent != nil {
				ref, ok := entry.(types.IndirectRef)
				if !ok {
					return errors.New("tagged PDF note does not use an indirect annotation")
				}
				parents[*parent] = struct{}{}
				converted[ref] = struct{}{}
			}
			kept = append(kept, entry)
		}
		if len(kept) != len(entries) {
			dict["Annots"] = kept
		}
	}
	if len(seen) != len(s.text) {
		return fmt.Errorf("PDF renderer placed %d of %d note markers", len(seen), len(s.text))
	}
	return retagNotes(ctx, pdf, parents, converted)
}

func markerURL(pdf *model.Context, annotation types.Dict) (string, error) {
	if subtype := annotation.NameEntry("Subtype"); subtype == nil || *subtype != "Link" || annotation["A"] == nil {
		return "", nil
	}
	action, err := pdf.DereferenceDict(annotation["A"])
	if err != nil {
		return "", err
	}
	if kind := action.NameEntry("S"); kind == nil || *kind != "URI" {
		return "", nil
	}
	url, err := action.StringOrHexLiteralEntry("URI")
	if url == nil {
		return "", err
	}
	return *url, err
}

type placement struct {
	page       int
	box        *types.Rectangle
	annotation types.Dict
}

func linkRectangle(pdf *model.Context, annotation types.Dict) (*types.Rectangle, error) {
	rect, err := pdf.DereferenceArray(annotation["Rect"])
	if err != nil || len(rect) != 4 {
		return nil, errors.New("missing PDF link rectangle")
	}
	for _, coord := range rect {
		switch coord.(type) {
		case types.Integer, types.Float:
		default:
			return nil, errors.New("invalid PDF link coordinate")
		}
	}
	box := types.RectForArray(rect)
	if box.UR.X <= box.LL.X || box.UR.Y <= box.LL.Y {
		return nil, errors.New("empty PDF link rectangle")
	}
	return box, nil
}

const noteSize = 6

func noteRect(box *types.Rectangle) types.Array {
	// Acrobat can show a comment badge above and to the left of the
	// markup. Leave room for it beside the status and keep its target
	// close to the center of the status line.
	left, top := box.UR.X+10, box.UR.Y-3
	return types.NewRectangle(left, top-noteSize, left+noteSize, top).Array()
}

func noteAppearance(pdf *model.Context) (*types.IndirectRef, error) {
	// A Square markup can display its compact appearance and still expose
	// a comment. Acrobat substitutes its own larger icon for Text notes.
	mark := []byte("q .35 G .55 w 1 1.9 m 1 5 l 5 5 l 5 2 l 3 2 l 1.7 .9 l 1.7 2 l h S Q")
	stream, err := pdf.NewStreamDictForBuf(mark)
	if err != nil {
		return nil, err
	}
	stream.InsertName("Type", "XObject")
	stream.InsertName("Subtype", "Form")
	stream.InsertInt("FormType", 1)
	stream.Dict["BBox"] = types.NewRectangle(0, 0, noteSize, noteSize).Array()
	stream.Dict["Resources"] = types.NewDict()
	if err := stream.Encode(); err != nil {
		return nil, err
	}
	return pdf.IndRefForNewObject(*stream)
}

func replaceLink(annotation types.Dict, text string, box *types.Rectangle, appearance *types.IndirectRef, hidden bool) {
	for key := range annotation {
		if key != "Type" && key != "P" && key != "StructParent" {
			delete(annotation, key)
		}
	}

	annotation.InsertName("Type", "Annot")
	annotation.InsertName("Subtype", "Square")
	annotation["Rect"] = noteRect(box)
	annotation["Contents"] = types.NewHexLiteral([]byte(types.EncodeUTF16String(text)))
	annotation["AP"] = types.Dict{"N": *appearance}
	if hidden {
		// Retain the Word object's structure reference for tagged fragments.
		annotation["F"] = types.Integer(2) // Hidden, including when printing.
	}
}
