package pdf

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/marcuoli/go-document-docx/publication"
	"github.com/marcuoli/go-document-pdf/internal/reportfonts"
)

type portableDOCX struct {
	publication.Source
	document  finalizedDOCX
	directory string
}

func (s portableDOCX) Write(ctx context.Context, w io.Writer) error {
	ctx = reportfonts.WithResolver(ctx, s.withFonts)
	return s.Source.Write(ctx, w)
}

func (s portableDOCX) withFonts(ctx context.Context, render func(reportfonts.Faces) error) (returnedErr error) {
	dir, err := os.MkdirTemp(s.directory, ".pdf-fonts-")
	if err != nil {
		return err
	}
	defer func() { returnedErr = errors.Join(returnedErr, os.RemoveAll(dir)) }()
	stage, err := os.CreateTemp(dir, "verified-*.docx")
	if err != nil {
		return err
	}
	defer func() { returnedErr = errors.Join(returnedErr, stage.Close()) }()
	if err := s.document.Write(ctx, stage); err != nil {
		return err
	}
	docx, err := zip.NewReader(stage, s.document.receipt.Size)
	if err != nil {
		return fmt.Errorf("read finalized DOCX for embedded fonts: %w", err)
	}
	faces, err := reportfonts.Stage(ctx, docx, dir)
	if err != nil {
		return err
	}
	return render(faces)
}
