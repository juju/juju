// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package transformer

import (
	"context"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/errors"
)

// Fake payload types. Each version gets its own named struct so the test
// exercises the type-assertion boundary in [NewTransformation].
type payloadA struct{ Val int }
type payloadB struct{ Val int }
type payloadC struct{ Val int }

type transformerSuite struct{}

func TestTransformerSuite(t *testing.T) {
	tc.Run(t, &transformerSuite{})
}

func version(v string) semversion.Number {
	return semversion.MustParse(v)
}

func versions(vs ...string) []semversion.Number {
	result := make([]semversion.Number, len(vs))
	for i, v := range vs {
		result[i] = version(v)
	}
	return result
}

func okAtoB(_ context.Context, src *payloadA) (*payloadB, error) {
	return &payloadB{Val: src.Val + 1}, nil
}

func okBtoC(_ context.Context, src *payloadB) (*payloadC, error) {
	return &payloadC{Val: src.Val + 10}, nil
}

// okAtoC has srcType *payloadA, not *payloadB, so it breaks the chain after okAtoB.
func okAtoC(_ context.Context, src *payloadA) (*payloadC, error) {
	return &payloadC{Val: src.Val}, nil
}

func failBtoC(_ context.Context, _ *payloadB) (*payloadC, error) {
	return nil, errors.Errorf("boom")
}

func (s *transformerSuite) TestNewRejectsEmptyVersions(c *tc.C) {
	_, err := NewTransformer(nil, nil)
	c.Assert(err, tc.ErrorMatches, "no export versions defined")
}

func (s *transformerSuite) TestNewSingleVersionIsValid(c *tc.C) {
	// One version = no transformations needed = a pure pass-through transformer.
	a, err := NewTransformer(nil, versions("1.0.0"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(a.Target(), tc.Equals, version("1.0.0"))
}

func (s *transformerSuite) TestNewDetectsLengthMismatch(c *tc.C) {
	// Zero transformations for two versions: length check fires before anything else.
	_, err := NewTransformer(nil, versions("1.0.0", "1.1.0"))
	c.Assert(err, tc.ErrorIs, ErrTransformerLengthMismatch)
}

func (s *transformerSuite) TestNewDetectsMissingTransformer(c *tc.C) {
	// Right count but "1.1.0" step is absent — "0.9.0" -> "1.2.0" fills the slot without covering it.
	transformations := []Transformation{
		NewTransformation("1.0.0", "1.1.0", okAtoB),
		NewTransformation("0.9.0", "1.2.0", okBtoC),
	}
	_, err := NewTransformer(transformations, versions("1.0.0", "1.1.0", "1.2.0"))
	c.Assert(err, tc.ErrorIs, ErrMissingTransformer)
}

func (s *transformerSuite) TestNewDetectsTypeMismatch(c *tc.C) {
	// okAtoB outputs *payloadB; okAtoC expects *payloadA — chain is broken.
	transformations := []Transformation{
		NewTransformation("1.0.0", "1.1.0", okAtoB),
		NewTransformation("1.1.0", "1.2.0", okAtoC),
	}
	_, err := NewTransformer(transformations, versions("1.0.0", "1.1.0", "1.2.0"))
	c.Assert(err, tc.ErrorIs, ErrTransformerTypeMismatch)
}

func (s *transformerSuite) TestNewDetectsWrongToInChain(c *tc.C) {
	// Transformation goes 1.0.0 -> 1.2.0 but versions expect 1.0.0 -> 1.1.0.
	reg := NewTransformation("1.0.0", "1.2.0", okAtoB)
	_, err := NewTransformer([]Transformation{reg}, versions("1.0.0", "1.1.0"))
	c.Assert(err, tc.ErrorIs, ErrMissingTransformer)
}

func (s *transformerSuite) TestNewDetectsDuplicateTransformer(c *tc.C) {
	reg1 := NewTransformation("1.0.0", "1.1.0", okAtoB)
	reg2 := NewTransformation("1.0.0", "1.1.0", okAtoB)
	// Two transformations for three versions satisfies the length check, but the
	// duplicate "from" is caught before the chain walk.
	_, err := NewTransformer([]Transformation{reg1, reg2}, versions("1.0.0", "1.1.0", "1.2.0"))
	c.Assert(err, tc.ErrorIs, ErrDuplicateTransformer)
}

func (s *transformerSuite) TestTransformPassesThroughWhenSrcIsTarget(c *tc.C) {
	a, err := NewTransformer(nil, versions("1.0.0"))
	c.Assert(err, tc.ErrorIsNil)

	payload := &payloadA{Val: 7}
	got, err := a.Transform(c.Context(), version("1.0.0"), payload)
	c.Assert(err, tc.ErrorIsNil)
	// Same pointer — no copy, no transformation ran.
	c.Check(got, tc.Equals, any(payload))
}

func (s *transformerSuite) TestTransformWalksChain(c *tc.C) {
	transformations := []Transformation{
		NewTransformation("1.0.0", "1.1.0", okAtoB),
		NewTransformation("1.1.0", "1.2.0", okBtoC),
	}
	a, err := NewTransformer(transformations, versions("1.0.0", "1.1.0", "1.2.0"))
	c.Assert(err, tc.ErrorIsNil)

	got, err := a.Transform(c.Context(), version("1.0.0"), &payloadA{Val: 1})
	c.Assert(err, tc.ErrorIsNil)
	// 1 + 1 (AtoB) + 10 (BtoC) = 12.
	c.Check(got, tc.DeepEquals, &payloadC{Val: 12})
}

func (s *transformerSuite) TestTransformRejectsUnknownSource(c *tc.C) {
	transformations := []Transformation{
		NewTransformation("1.0.0", "1.1.0", okAtoB),
	}
	a, err := NewTransformer(transformations, versions("1.0.0", "1.1.0"))
	c.Assert(err, tc.ErrorIsNil)

	_, err = a.Transform(c.Context(), version("0.9.0"), &payloadA{})
	c.Assert(err, tc.ErrorIs, ErrUnknownSourceVersion)
}

func (s *transformerSuite) TestTransformRejectsPayloadTypeMismatch(c *tc.C) {
	transformations := []Transformation{
		NewTransformation("1.0.0", "1.1.0", okAtoB),
	}
	a, err := NewTransformer(transformations, versions("1.0.0", "1.1.0"))
	c.Assert(err, tc.ErrorIsNil)

	// Transformation expects *payloadA, we hand it *payloadB.
	_, err = a.Transform(c.Context(), version("1.0.0"), &payloadB{})
	c.Assert(err, tc.ErrorIs, ErrPayloadTypeMismatch)
}

func (s *transformerSuite) TestTransformFromMidChain(c *tc.C) {
	transformations := []Transformation{
		NewTransformation("1.0.0", "1.1.0", okAtoB),
		NewTransformation("1.1.0", "1.2.0", okBtoC),
	}
	a, err := NewTransformer(transformations, versions("1.0.0", "1.1.0", "1.2.0"))
	c.Assert(err, tc.ErrorIsNil)

	// Starting from "1.1.0" should apply only the BtoC step, not AtoB.
	got, err := a.Transform(c.Context(), version("1.1.0"), &payloadB{Val: 5})
	c.Assert(err, tc.ErrorIsNil)
	// 5 + 10 (BtoC) = 15; AtoB (+1) must not have run.
	c.Check(got, tc.DeepEquals, &payloadC{Val: 15})
}

func (s *transformerSuite) TestTransformPassesThroughWhenSrcIsTargetInMultiStepChain(c *tc.C) {
	transformations := []Transformation{
		NewTransformation("1.0.0", "1.1.0", okAtoB),
		NewTransformation("1.1.0", "1.2.0", okBtoC),
	}
	a, err := NewTransformer(transformations, versions("1.0.0", "1.1.0", "1.2.0"))
	c.Assert(err, tc.ErrorIsNil)

	payload := &payloadC{Val: 99}
	got, err := a.Transform(c.Context(), version("1.2.0"), payload)
	c.Assert(err, tc.ErrorIsNil)
	// Same pointer — no transformation ran.
	c.Check(got, tc.Equals, any(payload))
}

func (s *transformerSuite) TestTransformNilPayloadReturnsMismatch(c *tc.C) {
	transformations := []Transformation{
		NewTransformation("1.0.0", "1.1.0", okAtoB),
	}
	a, err := NewTransformer(transformations, versions("1.0.0", "1.1.0"))
	c.Assert(err, tc.ErrorIsNil)

	// nil any fails the *payloadA type assertion inside the closure.
	_, err = a.Transform(c.Context(), version("1.0.0"), nil)
	c.Assert(err, tc.ErrorIs, ErrPayloadTypeMismatch)
}

func (s *transformerSuite) TestTransformWrapsMidChainErrors(c *tc.C) {
	transformations := []Transformation{
		NewTransformation("1.0.0", "1.1.0", okAtoB),
		NewTransformation("1.1.0", "1.2.0", failBtoC),
	}
	a, err := NewTransformer(transformations, versions("1.0.0", "1.1.0", "1.2.0"))
	c.Assert(err, tc.ErrorIsNil)

	_, err = a.Transform(c.Context(), version("1.0.0"), &payloadA{Val: 0})
	c.Assert(err, tc.ErrorMatches, "transforming 1.1.0 -> 1.2.0: boom")
}
