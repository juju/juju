// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package transformer

import (
	"context"

	"github.com/juju/juju/core/semversion"
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
	versions []semversion.Number
	// chain maps a source version to its transformation entry. The target
	// version has no entry (nothing to run).
	chain  map[semversion.Number]Transformation
	target semversion.Number
}

// NewTransformer builds a Transformer from the given transformations and the
// ordered list of schema format versions. Invoked at controller startup;
// returns an error if the chain is not well-formed.
func NewTransformer(transformations []Transformation, versions []semversion.Number) (*Transformer, error) {
	if len(versions) == 0 {
		return nil, errors.Errorf("no export versions defined")
	}

	// Check that only one transformation is defined per adjacent version pair.
	if err := validateStepCount(transformations, versions); err != nil {
		return nil, err
	}
	// Index by source version, and return a bespoke error if any duplicates
	// are found.
	chain, err := buildChain(transformations)
	if err != nil {
		return nil, err
	}

	// Check that every adjacent version pair has a corresponding transformation.
	if err := validateCompleteness(chain, versions); err != nil {
		return nil, err
	}

	// Check that each step's output type feeds the next step's input type.
	if err := validateTypeContinuity(chain, versions); err != nil {
		return nil, err
	}

	return &Transformer{
		versions: versions,
		chain:    chain,
		target:   versions[len(versions)-1],
	}, nil
}

// validateStepCount checks there is exactly one transformation per adjacent
// version pair: N versions require N-1 steps. Any other count means a step is
// missing or surplus - and a surplus transformation would be silently ignored
// by the chain walk rather than fail loudly - so it is rejected here with
// [ErrTransformerLengthMismatch].
func validateStepCount(transformations []Transformation, versions []semversion.Number) error {
	steps := len(versions) - 1
	if len(transformations) != steps {
		return errors.Errorf("need %d transformer(s) for %d version(s), got %d",
			steps, len(versions), len(transformations)).Add(ErrTransformerLengthMismatch)
	}
	return nil
}

// buildChain indexes the transformations by their source version so the walk
// can look up the next step in O(1). Two registrations sharing a source
// version would make the chain ambiguous (two possible steps out of one
// version), so a duplicate source is rejected with [ErrDuplicateTransformer]
// rather than letting one registration silently overwrite the other.
func buildChain(transformations []Transformation) (map[semversion.Number]Transformation, error) {
	chain := make(map[semversion.Number]Transformation, len(transformations))
	for _, transformation := range transformations {
		if _, dup := chain[transformation.from]; dup {
			return nil, errors.Errorf("duplicate transformer for version pair %q -> %q",
				transformation.from, transformation.to).Add(ErrDuplicateTransformer)
		}
		chain[transformation.from] = transformation
	}
	return chain, nil
}

// validateCompleteness checks the chain contains a transformation for every
// adjacent (versions[i] -> versions[i+1]) pair, with a matching destination.
// This guarantees a payload can be walked from any known version up to the
// target without hitting a gap. A missing or wrong-destination step is rejected
// with [ErrMissingTransformer]. Must run after [buildChain] so every expected
// source version is present in the map.
func validateCompleteness(chain map[semversion.Number]Transformation, versions []semversion.Number) error {
	steps := len(versions) - 1
	for i := range steps {
		from, to := versions[i], versions[i+1]
		transformation, ok := chain[from]
		if !ok || transformation.to != to {
			return errors.Errorf("missing transformer for version pair %q -> %q",
				from, to).Add(ErrMissingTransformer)
		}
	}
	return nil
}

// validateTypeContinuity checks each step's output Go type equals the next
// step's input Go type. The chain is type-erased (see [Transformation]), so a
// break here is invisible to the compiler and would otherwise surface only at
// runtime as a payload type-assertion failure mid-walk; verifying it up front
// turns that into a construction-time [ErrTransformerTypeMismatch]. Must run
// after [validateCompleteness], which guarantees every indexed step exists.
func validateTypeContinuity(chain map[semversion.Number]Transformation, versions []semversion.Number) error {
	steps := len(versions) - 1
	for i := 0; i < steps-1; i++ {
		currentTransformation, nextTransformation := chain[versions[i]], chain[versions[i+1]]
		if currentTransformation.dstType != nextTransformation.srcType {
			return errors.Errorf(
				"type mismatch at %q -> %q: outputs %s but %q -> %q expects %s",
				versions[i], versions[i+1], currentTransformation.dstType,
				versions[i+1], versions[i+2], nextTransformation.srcType,
			).Add(ErrTransformerTypeMismatch)
		}
	}
	return nil
}

// Transform walks payload forward from srcVersion to the transformer's target
// version, applying one registered transformation per step. Each step's
// expected Src type is verified against payload's runtime type before
// invocation (see [NewTransformation]). If any step fails, the returned error
// is wrapped with the failing (from -> to) pair.
//
// If srcVersion equals the target, payload is returned unchanged.
func (t *Transformer) Transform(ctx context.Context, srcVersion semversion.Number, payload any) (any, error) {
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
func (t *Transformer) Target() semversion.Number {
	return t.target
}

// versionIndex returns the index of version v in t.versions, or -1 if not
// found.
func (t *Transformer) versionIndex(v semversion.Number) int {
	for i, x := range t.versions {
		if x == v {
			return i
		}
	}
	return -1
}
