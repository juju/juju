// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package transformer

import "context"

// TransformationFunc converts a payload of schema format version Src into a
// payload of schema format version Dst. Implementations are produced per
// version step by a generator-emitted transform.go plus an engineer-written
// deltas.go (see domain/modelimport/transformer/transforms/<pair>).
type TransformationFunc[Src, Dst any] func(ctx context.Context, src *Src) (*Dst, error)
