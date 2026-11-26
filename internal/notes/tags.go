package notes

import (
	"context"
	"errors"
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Retain the exported marked text and object references. A Word Link structure
// element that now owns a text annotation becomes an Annot structure element.
func retagNotes(ctx context.Context, pdf *model.Context, parents map[int]struct{}, converted map[types.IndirectRef]struct{}) error {
	if len(parents) == 0 {
		return nil
	}
	catalog, err := pdf.Catalog()
	if err != nil {
		return err
	}
	tree, err := pdf.DereferenceDict(catalog["StructTreeRoot"])
	if err != nil || tree == nil || tree["ParentTree"] == nil {
		return errors.New("missing PDF structure parent tree for notes")
	}
	nodes := []types.Object{tree["ParentTree"]}
	visited := make(map[types.IndirectRef]bool)
	remaining := len(parents)
	for len(nodes) > 0 && remaining > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		last := len(nodes) - 1
		ref := nodes[last]
		nodes = nodes[:last]
		if indirect, ok := ref.(types.IndirectRef); ok {
			if visited[indirect] {
				return errors.New("repeated PDF structure parent tree node")
			}
			visited[indirect] = true
		}
		node, err := pdf.DereferenceDict(ref)
		if err != nil {
			return err
		}
		children, err := pdf.DereferenceArray(node["Kids"])
		if err != nil {
			return err
		}
		nodes = append(nodes, children...)
		numbers, err := pdf.DereferenceArray(node["Nums"])
		if err != nil {
			return err
		}
		if len(numbers)%2 != 0 {
			return errors.New("invalid PDF structure parent tree entries")
		}
		for i := 0; i < len(numbers); i += 2 {
			parent, ok := numbers[i].(types.Integer)
			if !ok {
				return errors.New("invalid PDF structure parent index")
			}
			if _, found := parents[int(parent)]; !found {
				continue
			}
			owner, err := pdf.DereferenceDict(numbers[i+1])
			if err != nil {
				return err
			}
			if err := retagNoteOwner(pdf, owner, converted); err != nil {
				return fmt.Errorf("retag PDF note parent %d: %w", parent, err)
			}
			remaining--
		}
	}
	if remaining != 0 {
		return errors.New("unmapped tagged PDF note")
	}
	return nil
}

func retagNoteOwner(pdf *model.Context, owner types.Dict, converted map[types.IndirectRef]struct{}) error {
	role := owner.NameEntry("S")
	if role == nil || (*role != "Link" && *role != "Annot") {
		return errors.New("unexpected structure type for converted PDF link")
	}
	content, err := pdf.Dereference(owner["K"])
	if err != nil {
		return err
	}
	children, ok := content.(types.Array)
	if ok {
		children = append(types.Array(nil), children...)
	} else {
		children = types.Array{content}
	}
	found := false
	visited := map[types.IndirectRef]bool{}
	for i := 0; i < len(children); i++ {
		child := children[i]
		if ref, ok := child.(types.IndirectRef); ok {
			if visited[ref] {
				return errors.New("repeated child in a PDF link structure element")
			}
			visited[ref] = true
		}
		item, err := pdf.Dereference(child)
		if err != nil {
			return err
		}
		if _, marked := item.(types.Integer); marked {
			continue
		}
		dict, ok := item.(types.Dict)
		if !ok {
			return errors.New("unexpected content in a PDF link structure element")
		}
		kind := dict.NameEntry("Type")
		if kind != nil {
			switch *kind {
			case "MCR":
				continue
			case "StructElem":
				nested, err := pdf.Dereference(dict["K"])
				if err != nil {
					return err
				}
				if entries, ok := nested.(types.Array); ok {
					children = append(children, entries...)
				} else if nested != nil {
					children = append(children, nested)
				}
				continue
			}
		}
		if kind == nil || *kind != "OBJR" {
			return errors.New("unexpected content in a PDF link structure element")
		}
		ref, ok := dict["Obj"].(types.IndirectRef)
		if _, converted := converted[ref]; !ok || !converted {
			return errors.New("PDF annotation structure shares an authored link")
		}
		found = true
	}
	if !found {
		return errors.New("missing annotation in PDF structure element")
	}
	owner["S"] = types.Name("Annot")
	return nil
}
