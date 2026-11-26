package pdf

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/marcuoli/go-document-docx/publication"
	"github.com/marcuoli/go-document-docx/word"
)

// SaveFromDOCX publishes a PDF at outputPath. On Windows it exports the DOCX
// only after verifying the exact staged bytes against its publication receipt.
// Other hosts use portable, as does Windows if Word is unavailable before
// writing. The input and output paths must be distinct.
func SaveFromDOCX(ctx context.Context, outputPath, docxPath string, receipt publication.Receipt, portable publication.Source, overwrite bool) (Receipt, error) {
	output, err := filepath.Abs(outputPath)
	if err != nil {
		return Receipt{}, err
	}
	input, err := filepath.Abs(docxPath)
	if err != nil {
		return Receipt{}, err
	}
	if input == output || runtime.GOOS == "windows" && strings.EqualFold(input, output) {
		return Receipt{}, errDOCXDestination
	}
	// Path spelling alone cannot detect aliases through a directory
	// symlink, junction or hard link.
	if info, err := os.Stat(output); err == nil {
		if original, err := os.Stat(input); err == nil && os.SameFile(info, original) {
			return Receipt{}, errDOCXDestination
		}
	}
	document := finalizedDOCX{path: input, receipt: receipt}
	directory := filepath.Dir(output)
	source := word.PDFPreservingLinks(docxWithoutScreenTips{document: document, directory: directory}, word.Options{TemporaryDirectory: directory})
	if portable != nil {
		portable = portableDOCX{Source: portable, document: document, directory: directory}
	}
	return Save(ctx, output, overwrite, Sources{WordPDF: source, PortablePDF: portable})
}

var errDOCXDestination = errors.New("PDF output must differ from the DOCX input")

var errDocumentChanged = errors.New("finalized DOCX differs from its publication receipt")

type finalizedDOCX struct {
	path    string
	receipt publication.Receipt
}

func (source finalizedDOCX) Write(ctx context.Context, output io.Writer) (returnedErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !source.receipt.Installed || source.receipt.Size <= 0 || source.receipt.SHA512 == "" {
		return errors.New("finalized DOCX publication receipt is required")
	}
	info, err := os.Lstat(source.path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() != source.receipt.Size {
		return errDocumentChanged
	}
	file, err := os.Open(source.path)
	if err != nil {
		return err
	}
	defer func() { returnedErr = errors.Join(returnedErr, file.Close()) }()
	opened, err := file.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(info, opened) || opened.Size() != source.receipt.Size {
		return errDocumentChanged
	}
	// Word opens its private stage only after this source returns successfully.
	// Hash exactly the bytes copied there, including any same-size replacement.
	digest := sha512.New()
	written, err := io.Copy(io.MultiWriter(output, digest), io.LimitReader(file, source.receipt.Size+1))
	if err != nil {
		return err
	}
	if written != source.receipt.Size || hex.EncodeToString(digest.Sum(nil)) != source.receipt.SHA512 {
		return errDocumentChanged
	}
	return ctx.Err()
}
