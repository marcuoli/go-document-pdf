package pdf

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/marcuoli/go-document-docx/publication"
	"github.com/marcuoli/go-document-docx/word"
)

type sourceFunc func(context.Context, io.Writer) error

func (f sourceFunc) Write(ctx context.Context, w io.Writer) error { return f(ctx, w) }

func TestBackendSelection(t *testing.T) {
	t.Parallel()
	conversionErr := errors.New("conversion failed")
	for _, test := range []struct {
		name       string
		windows    bool
		wordWrites bool
		wordErr    error
		want       string
		backend    Backend
		err        error
	}{
		{name: "Word", windows: true, wordWrites: true, want: "word", backend: Word},
		{name: "Windows without Word", windows: true, wordErr: word.ErrUnavailable, want: "portable", backend: Portable},
		{name: "portable host", wordWrites: true, want: "portable", backend: Portable},
		{name: "conversion failure", windows: true, wordErr: conversionErr, err: conversionErr},
		{name: "native wrote before failure", windows: true, wordWrites: true, wordErr: word.ErrUnavailable, want: "word", err: word.ErrUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := &selectedSource{
				preferWord: test.windows,
				sources: Sources{
					WordPDF: sourceFunc(func(_ context.Context, w io.Writer) error {
						if test.wordWrites {
							if _, err := io.WriteString(w, "word"); err != nil {
								return err
							}
						}
						return test.wordErr
					}),
					PortablePDF: sourceFunc(func(_ context.Context, w io.Writer) error {
						_, err := io.WriteString(w, "portable")
						return err
					}),
				},
			}
			var output strings.Builder
			err := source.Write(t.Context(), &output)
			if !errors.Is(err, test.err) || output.String() != test.want || source.backend != test.backend {
				t.Fatalf("backend=%s output=%q error=%v", source.backend, output.String(), err)
			}
		})
	}
}

func TestSaveRetainsDestinationWhenRenderFails(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "report.pdf")
	if err := os.WriteFile(path, []byte("existing report"), 0600); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("portable render failed")
	broken := sourceFunc(func(_ context.Context, w io.Writer) error {
		_, err := io.WriteString(w, "partial new report")
		return errors.Join(failure, err)
	})
	result, err := Save(t.Context(), path, true, Sources{PortablePDF: broken})
	if !errors.Is(err, failure) || result.Installed || result.Backend != "" {
		t.Fatalf("unexpected failed publication: %+v %v", result, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "existing report" {
		t.Fatalf("old report changed: %q %v", data, err)
	}
	result, err = Save(t.Context(), path, true, Sources{PortablePDF: publication.FromBytes([]byte("new PDF"))})
	if err != nil || !result.Installed || result.Backend != Portable || result.Size != 7 || result.SHA512 == "" {
		t.Fatalf("unexpected installed receipt: %+v %v", result, err)
	}
}
