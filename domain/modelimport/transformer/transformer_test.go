// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package transformer

import (
	"context"
	"testing"

	"github.com/juju/tc"

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

func okAtoB(_ context.Context, src *payloadA) (*payloadB, error) {
	return &payloadB{Val: src.Val + 1}, nil
}

func okBtoC(_ context.Context, src *payloadB) (*payloadC, error) {
	return &payloadC{Val: src.Val + 10}, nil
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
	a, err := NewTransformer(nil, []string{"1.0"})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(a.Target(), tc.Equals, "1.0")
}

func (s *transformerSuite) TestNewDetectsMissingTransformer(c *tc.C) {
	_, err := NewTransformer(nil, []string{"1.0", "1.1"})
	c.Assert(err, tc.ErrorIs, ErrMissingTransformer)
}

func (s *transformerSuite) TestNewDetectsWrongToInChain(c *tc.C) {
	// Transformation goes 1.0 -> 1.2 but versions expect 1.0 -> 1.1.
	reg := NewTransformation("1.0", "1.2", okAtoB)
	_, err := NewTransformer([]Transformation{reg}, []string{"1.0", "1.1"})
	c.Assert(err, tc.ErrorIs, ErrMissingTransformer)
}

func (s *transformerSuite) TestNewDetectsDuplicateTransformer(c *tc.C) {
	reg1 := NewTransformation("1.0", "1.1", okAtoB)
	reg2 := NewTransformation("1.0", "1.1", okAtoB)
	_, err := NewTransformer([]Transformation{reg1, reg2}, []string{"1.0", "1.1"})
	c.Assert(err, tc.ErrorIs, ErrDuplicateTransformer)
}

func (s *transformerSuite) TestTransformPassesThroughWhenSrcIsTarget(c *tc.C) {
	a, err := NewTransformer(nil, []string{"1.0"})
	c.Assert(err, tc.ErrorIsNil)

	payload := &payloadA{Val: 7}
	got, err := a.Transform(c.Context(), "1.0", payload)
	c.Assert(err, tc.ErrorIsNil)
	// Same pointer — no copy, no transformation ran.
	c.Check(got, tc.Equals, any(payload))
}

func (s *transformerSuite) TestTransformWalksChain(c *tc.C) {
	regs := []Transformation{
		NewTransformation("1.0", "1.1", okAtoB),
		NewTransformation("1.1", "1.2", okBtoC),
	}
	a, err := NewTransformer(regs, []string{"1.0", "1.1", "1.2"})
	c.Assert(err, tc.ErrorIsNil)

	got, err := a.Transform(c.Context(), "1.0", &payloadA{Val: 1})
	c.Assert(err, tc.ErrorIsNil)
	// 1 + 1 (AtoB) + 10 (BtoC) = 12.
	c.Check(got, tc.DeepEquals, &payloadC{Val: 12})
}

func (s *transformerSuite) TestTransformRejectsUnknownSource(c *tc.C) {
	regs := []Transformation{
		NewTransformation("1.0", "1.1", okAtoB),
	}
	a, err := NewTransformer(regs, []string{"1.0", "1.1"})
	c.Assert(err, tc.ErrorIsNil)

	_, err = a.Transform(c.Context(), "0.9", &payloadA{})
	c.Assert(err, tc.ErrorIs, ErrUnknownSourceVersion)
}

func (s *transformerSuite) TestTransformRejectsPayloadTypeMismatch(c *tc.C) {
	regs := []Transformation{
		NewTransformation("1.0", "1.1", okAtoB),
	}
	a, err := NewTransformer(regs, []string{"1.0", "1.1"})
	c.Assert(err, tc.ErrorIsNil)

	// Transformation expects *payloadA, we hand it *payloadB.
	_, err = a.Transform(c.Context(), "1.0", &payloadB{})
	c.Assert(err, tc.ErrorIs, ErrPayloadTypeMismatch)
}

func (s *transformerSuite) TestTransformWrapsMidChainErrors(c *tc.C) {
	regs := []Transformation{
		NewTransformation("1.0", "1.1", okAtoB),
		NewTransformation("1.1", "1.2", failBtoC),
	}
	a, err := NewTransformer(regs, []string{"1.0", "1.1", "1.2"})
	c.Assert(err, tc.ErrorIsNil)

	_, err = a.Transform(c.Context(), "1.0", &payloadA{Val: 0})
	c.Assert(err, tc.ErrorMatches, "transforming 1.1 -> 1.2: boom")
}
