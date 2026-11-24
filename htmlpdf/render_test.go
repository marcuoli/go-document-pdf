package htmlpdf

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestRenderOwnsInputAndHonorsCancellation(t *testing.T) {
	t.Parallel()
	html := []byte("<h1 id='summary'>Summary</h1><p><a href='#details'>Read details</a></p><h2 id='details'>Details</h2>")
	src := New(html, Options{Title: "Report", ContentsTitle: "Contents"})
	clear(html)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var canceled bytes.Buffer
	if err := src.Write(ctx, &canceled); !errors.Is(err, context.Canceled) || canceled.Len() != 0 {
		t.Fatalf("canceled render wrote %d bytes with error %v", canceled.Len(), err)
	}

	var output bytes.Buffer
	if err := src.Write(t.Context(), &output); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(output.Bytes(), []byte("%PDF-")) ||
		!bytes.Contains(output.Bytes(), []byte("/Outlines")) ||
		!bytes.Contains(output.Bytes(), []byte("/Subtype /Link")) {
		t.Fatal("PDF signature, outline, or link annotation is missing")
	}
}
