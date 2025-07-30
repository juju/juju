// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"slices"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"
	"golang.org/x/sync/errgroup"

	"github.com/juju/juju/domain"
	schematesting "github.com/juju/juju/domain/schema/testing"
	domainsequence "github.com/juju/juju/domain/sequence"
)

type sequenceSuite struct {
	schematesting.ModelSuite

	state *domain.StateBase
}

func TestSequenceSuite(t *testing.T) {
	tc.Run(t, &sequenceSuite{})
}

func (s *sequenceSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)
	s.state = domain.NewStateBase(s.TxnRunnerFactory())
}

func (s *sequenceSuite) TestSequenceStaticNamespace(c *tc.C) {
	var next uint64
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		next, err = NextValue(ctx, s.state, tx, domainsequence.StaticNamespace("foo"))
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(next, tc.Equals, uint64(0))

	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		next, err = NextValue(ctx, s.state, tx, domainsequence.StaticNamespace("foo"))
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(next, tc.Equals, uint64(1))
}

func (s *sequenceSuite) TestSequencePrefixNamespace(c *tc.C) {
	var next uint64
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		next, err = NextValue(ctx, s.state, tx, domainsequence.MakePrefixNamespace(domainsequence.StaticNamespace("foo"), "bar"))
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(next, tc.Equals, uint64(0))

	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		next, err = NextValue(ctx, s.state, tx, domainsequence.MakePrefixNamespace(domainsequence.StaticNamespace("foo"), "bar"))
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(next, tc.Equals, uint64(1))
}

func (s *sequenceSuite) TestSequenceMultiple(c *tc.C) {
	const n = 100
	buf := make(chan uint64, n)

	eg, egCtx := errgroup.WithContext(c.Context())
	for range n {
		eg.Go(func() error {
			var next uint64
			err := s.TxnRunner().Txn(egCtx, func(ctx context.Context, tx *sqlair.TX) error {
				var err error
				next, err = NextValue(ctx, s.state, tx, domainsequence.StaticNamespace("foo"))
				return err
			})
			buf <- next
			return err
		})
	}
	err := eg.Wait()
	c.Assert(err, tc.ErrorIsNil)

	values := make([]uint64, 0, n)
	for range n {
		values = append(values, <-buf)
	}
	slices.Sort(values)

	first := true
	last := uint64(0)
	for _, next := range values {
		if first {
			first = false
			if next != 0 {
				c.Fatal("sequence did not start with 0")
			}
			continue
		}
		c.Assert(next, tc.Equals, last+1)
		last = next
	}
}

// TestSequenceNValues tests that the [NextNValues] can be used to generate n
// number of new sequence numbers.
func (s *sequenceSuite) TestSequenceNValues(c *tc.C) {
	var next []uint64
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		next, err = NextNValues(ctx, s.state, tx, 10, domainsequence.StaticNamespace("foo"))
		return err
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(next, tc.SameContents, []uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})

	// Run again and check that the next sequewnces are addative.
	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		next, err = NextNValues(ctx, s.state, tx, 3, domainsequence.StaticNamespace("foo"))
		return err
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(next, tc.SameContents, []uint64{10, 11, 12})
}

// TestSequenceNValuesOne tests that the [NextNValues] can be used to generate 1
// number of new sequence numbers.
func (s *sequenceSuite) TestSequenceNValuesOne(c *tc.C) {
	var next []uint64
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		next, err = NextNValues(ctx, s.state, tx, 1, domainsequence.StaticNamespace("foo"))
		return err
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(next, tc.SameContents, []uint64{0})
}

// TestSequenceNValuesZero tests that [NextNValues] when called with a zero
// value does not use up an sequence numbers in the namespace.
func (s *sequenceSuite) TestSequenceNValuesZero(c *tc.C) {
	var next []uint64
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		next, err = NextNValues(ctx, s.state, tx, 0, domainsequence.StaticNamespace("foo"))
		return err
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(next, tc.HasLen, 0)

	// Test that the next sequence number is zero proving nothing was use up.
	var current uint64
	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		current, err = NextValue(ctx, s.state, tx, domainsequence.StaticNamespace("foo"))
		return err
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(current, tc.Equals, uint64(0))
}
