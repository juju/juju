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
// duplicate step, length mismatch, type chain break, or no versions configured.
func NewTransformer(transformations []Transformation, versions []string) (*Transformer, error) {
	if len(versions) == 0 {
		return nil, errors.Errorf("no export versions defined")
	}

	steps := len(versions) - 1
	if len(transformations) != steps {
		return nil, errors.Errorf("need %d transformer(s) for %d version(s), got %d",
			steps, len(versions), len(transformations)).Add(ErrTransformerLengthMismatch)
	}

	chain := make(map[string]Transformation, len(transformations))
	for _, transformation := range transformations {
		if _, dup := chain[transformation.from]; dup {
			return nil, errors.Errorf("duplicate transformer for version pair %q -> %q",
				transformation.from, transformation.to).Add(ErrDuplicateTransformer)
		}
		chain[transformation.from] = transformation
	}

	// Verify transformation chain completeness: each version must have a
	// corresponding transformation.
	for i := range steps {
		from, to := versions[i], versions[i+1]
		transformation, ok := chain[from]
		if !ok || transformation.to != to {
			return nil, errors.Errorf("missing transformer for version pair %q -> %q",
				from, to).Add(ErrMissingTransformer)
		}
	}

	// Verify type chain continuity: each step's output type must equal the
	// next step's input type, or a runtime type-assertion failure is guaranteed.
	for i := 0; i < steps-1; i++ {
		currentTransformation, nextTransformation := chain[versions[i]], chain[versions[i+1]]
		if currentTransformation.dstType != nextTransformation.srcType {
			return nil, errors.Errorf(
				"type mismatch at %q -> %q: outputs %s but %q -> %q expects %s",
				versions[i], versions[i+1], currentTransformation.dstType,
				versions[i+1], versions[i+2], nextTransformation.srcType,
			).Add(ErrTransformerTypeMismatch)
		}
	}

	return &Transformer{
		versions: versions,
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

	if t.versionIndex(srcVersion) < 0 {
		return nil, errors.Errorf("unknown source export version: %q", srcVersion).Add(ErrUnknownSourceVersion)
	}

	current := srcVersion
	currentPayload := payload
	for current != t.target {
		transformation, ok := t.chain[current]
		if !ok {
			return nil, errors.Errorf("missing version in transformation chain: %q", current).Add(ErrMissingTransformer)
		}
		nextPayload, err := transformation.transform(ctx, currentPayload)
		if err != nil {
			return nil, errors.Errorf("transforming %s -> %s: %w", transformation.from, transformation.to, err)
		}
		currentPayload = nextPayload
		current = transformation.to
	}
	return currentPayload, nil
}

// Target returns the schema format version this transformer walks payloads up to.
func (t *Transformer) Target() string {
	return t.target
}

// versionIndex returns the index of version v in t.versions, or -1 if not
// found.
func (t *Transformer) versionIndex(v string) int {
	for i, x := range t.versions {
		if x == v {
			return i
		}
	}
	return -1
}
