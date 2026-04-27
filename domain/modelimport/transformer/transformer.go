// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package transformer

import (
	"context"

	"github.com/juju/juju/internal/errors"
)

// Transformer walks a payload through a chain of version-to-version
// [Transformation]s to bring it up to a target schema format version.
//
// The chain is a linear sequence of [Transformation] values, one per
// adjacent pair in a caller-supplied version list. [NewTransformer]
// validates the chain at construction so the controller refuses to start
// when a step is missing.
type Transformer struct {
	// versions is the ordered list of schema format versions the transformer
	// knows about. The last entry is the target.
	versions []string
	// chain maps a source version to its transformation entry. The target
	// version has no entry (nothing to run).
	chain  map[string]Transformation
	target string
}

// NewTransformer builds a Transformer from the given transformations and the
// ordered list of schema format versions. Invoked at controller startup;
// returns an error if the chain is not well-formed: missing step,
// duplicate step, or no versions configured.
func NewTransformer(regs []Transformation, versions []string) (*Transformer, error) {
	if len(versions) == 0 {
		return nil, errors.Errorf("no export versions defined")
	}

	chain := make(map[string]Transformation, len(regs))
	for _, r := range regs {
		if _, dup := chain[r.from]; dup {
			return nil, errors.Errorf("%w: %s -> %s", ErrDuplicateTransformer, r.from, r.to)
		}
		chain[r.from] = r
	}

	for i := 0; i < len(versions)-1; i++ {
		from, to := versions[i], versions[i+1]
		r, ok := chain[from]
		if !ok || r.to != to {
			return nil, errors.Errorf("%w: %s -> %s", ErrMissingTransformer, from, to)
		}
	}

	return &Transformer{
		versions: append([]string(nil), versions...),
		chain:    chain,
		target:   versions[len(versions)-1],
	}, nil
}

// Transform walks payload forward from srcVersion to the transformer's target
// version, applying one registered transformation per step. Each step's
// expected Src type is verified against payload's runtime type before
// invocation (see [NewTransformation]). If any step fails, the returned error
// is wrapped with the failing (from -> to) pair.
//
// If srcVersion equals the target, payload is returned unchanged.
func (t *Transformer) Transform(ctx context.Context, srcVersion string, payload any) (any, error) {
	if srcVersion == t.target {
		return payload, nil
	}

	if t.indexOf(srcVersion) < 0 {
		return nil, errors.Errorf("%w: %s", ErrUnknownSourceVersion, srcVersion)
	}

	current := srcVersion
	cur := payload
	for current != t.target {
		r, ok := t.chain[current]
		if !ok {
			return nil, errors.Errorf("%w: %s -> ?", ErrMissingTransformer, current)
		}
		next, err := r.transform(ctx, cur)
		if err != nil {
			return nil, errors.Errorf("transforming %s -> %s: %w", r.from, r.to, err)
		}
		cur = next
		current = r.to
	}
	return cur, nil
}

// Target returns the schema format version this transformer walks payloads up to.
func (t *Transformer) Target() string {
	return t.target
}

func (t *Transformer) indexOf(v string) int {
	for i, x := range t.versions {
		if x == v {
			return i
		}
	}
	return -1
}
