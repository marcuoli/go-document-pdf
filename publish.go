// Package pdf publishes PDF output using an optional Windows Word exporter
// and a caller-selected portable renderer.
package pdf

import (
	"context"
	"errors"
	"io"
	"runtime"

	"github.com/marcuoli/go-document-docx/publication"
	"github.com/marcuoli/go-document-docx/word"
)

// Backend identifies the renderer that produced an installed PDF.
type Backend string

const (
	// Word is available when the supplied desktop Word exporter can start.
	Word Backend = "word"
	// Portable is the supplied platform-independent renderer.
	Portable Backend = "portable"
)

// Sources supplies PDF producers. Both are optional individually. WordPDF is
// considered only on Windows. PortablePDF can produce the same report through
// its own layout. Each producer must emit a complete PDF or return an error.
type Sources struct {
	WordPDF     publication.Source
	PortablePDF publication.Source
}

// Receipt records the selected backend and the committed file's size and digest.
// Backend is empty if the PDF was not installed.
type Receipt struct {
	publication.Receipt
	Backend Backend
}

// ErrNoRenderer means there is no applicable PDF source for the host.
var ErrNoRenderer = errors.New("no PDF renderer supplied for this platform")

// Save stages and atomically installs a PDF. It uses the portable source when
// Word is absent or its exporter returns word.ErrUnavailable before writing.
// A conversion error after the native exporter starts writing is returned to
// the caller. Both backends share one destination transaction.
func Save(ctx context.Context, path string, overwrite bool, sources Sources) (Receipt, error) {
	source := &selectedSource{sources: sources, preferWord: runtime.GOOS == "windows"}
	receipt, err := publication.Save(ctx, path, overwrite, source)
	result := Receipt{Receipt: receipt}
	if receipt.Installed {
		result.Backend = source.backend
	}
	return result, err
}

type selectedSource struct {
	sources    Sources
	preferWord bool
	backend    Backend
}

func (s *selectedSource) Write(ctx context.Context, w io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.preferWord && s.sources.WordPDF != nil {
		output := &writeAttempt{Writer: w}
		err := s.sources.WordPDF.Write(ctx, output)
		if err == nil {
			s.backend = Word
			return nil
		}
		if output.started || !errors.Is(err, word.ErrUnavailable) || s.sources.PortablePDF == nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	if s.sources.PortablePDF == nil {
		return ErrNoRenderer
	}
	if err := s.sources.PortablePDF.Write(ctx, w); err != nil {
		return err
	}
	s.backend = Portable
	return nil
}

// A native producer that has touched the staging stream cannot switch to the
// portable producer, even when it reports an availability error.
type writeAttempt struct {
	io.Writer
	started bool
}

func (w *writeAttempt) Write(p []byte) (int, error) {
	w.started = w.started || len(p) > 0
	return w.Writer.Write(p)
}
