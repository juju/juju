// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package transformer

import (
	"context"
	"reflect"

	"github.com/juju/juju/internal/errors"
)

// Transformation is the type-erased form of a single version-to-version
// step. Construct instances with [NewTransformation]; pass a slice of them
// to [NewTransformer] to build a [Transformer]. Exported so top-level
// wiring packages can hold the registered list without an import cycle.
type Transformation struct {
	from, to  string
	srcType   reflect.Type // *Src
	dstType   reflect.Type // *Dst
	transform func(ctx context.Context, src any) (any, error)
}

// NewTransformation wraps a typed [TransformationFunc] into a [Transformation]
// entry. Storage erases the generic type parameters; the returned closure
// checks the payload's runtime Go type against Src before invoking fn so the
// erasure boundary stays safe.
func NewTransformation[Src, Dst any](from, to string, fn TransformationFunc[Src, Dst]) Transformation {
	var zeroSrc Src
	var zeroDst Dst
	expected := reflect.TypeOf(&zeroSrc)
	return Transformation{
		from:    from,
		to:      to,
		srcType: expected,
		dstType: reflect.TypeOf(&zeroDst),
		transform: func(ctx context.Context, src any) (any, error) {
			typed, ok := src.(*Src)
			if !ok {
				return nil, errors.Errorf("%w: expected %s, got %T",
					ErrPayloadTypeMismatch, expected, src)
			}
			return fn(ctx, typed)
		},
	}
}
