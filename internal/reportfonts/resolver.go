package reportfonts

import "context"

const (
	Symbol = "Segoe UI Symbol"
	Emoji  = "Segoe UI Emoji"
)

// Faces describes the embedded fonts admitted for one portable render.
type Faces struct {
	Directory string
	Symbol    bool
	Emoji     bool
}

// Resolver keeps the staged fonts available for the duration of render.
type Resolver func(ctx context.Context, render func(Faces) error) error

type resolverKey struct{}

// WithResolver carries export-local font access through deferred report sources.
func WithResolver(ctx context.Context, resolve Resolver) context.Context {
	return context.WithValue(ctx, resolverKey{}, resolve)
}

// Render uses embedded faces when supplied by SaveFromDOCX.
func Render(ctx context.Context, render func(Faces) error) error {
	if resolve, ok := ctx.Value(resolverKey{}).(Resolver); ok {
		return resolve(ctx, render)
	}
	return render(Faces{})
}
