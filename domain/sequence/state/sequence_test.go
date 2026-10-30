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
	"github.com/juju/juju/internal/errors"
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

// TestSequenceMixedAllocation checks ordered batches, zero-count no-ops and
// namespace isolation across single and batch allocations in one transaction.
func (s *sequenceSuite) TestSequenceMixedAllocation(c *tc.C) {
	for _, namespace := range []domainsequence.Namespace{
		domainsequence.StaticNamespace("foo"),
		domainsequence.MakePrefixNamespace(domainsequence.StaticNamespace("foo"), "bar"),
		domainsequence.StaticNamespace(""),
	} {
		var first, last uint64
		var batch, empty []uint64
		err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
			var err error
			first, err = NextValue(ctx, s.state, tx, namespace)
			if err != nil {
				return err
			}
			batch, err = NextNValues(ctx, s.state, tx, 3, namespace)
			if err != nil {
				return err
			}
			empty, err = NextNValues(ctx, s.state, tx, 0, namespace)
			if err != nil {
				return err
			}
			last, err = NextValue(ctx, s.state, tx, namespace)
			return err
		})
		c.Assert(err, tc.ErrorIsNil)
		c.Check(first, tc.Equals, uint64(0))
		c.Check(batch, tc.DeepEquals, []uint64{1, 2, 3})
		c.Check(empty, tc.IsNil)
		c.Check(last, tc.Equals, uint64(4))

		err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
			var err error
			batch, err = NextNValues(ctx, s.state, tx, 2, namespace)
			return err
		})
		c.Assert(err, tc.ErrorIsNil)
		c.Check(batch, tc.DeepEquals, []uint64{5, 6})
	}

	stored, err := NewState(s.TxnRunnerFactory()).GetSequencesForExport(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(stored, tc.DeepEquals, map[string]uint64{"foo": 6, "foo_bar": 6, "": 6})
}

// TestSequenceAllocationRollback checks that both inserts and increments are
// undone if the caller's transaction fails, for single and batch allocations.
func (s *sequenceSuite) TestSequenceAllocationRollback(c *tc.C) {
	state := NewState(s.TxnRunnerFactory())
	rollback := errors.New("rollback allocation")
	for _, test := range []struct {
		namespace string
		seeded    bool
		batch     bool
	}{
		{namespace: "new-single"},
		{namespace: "new-batch", batch: true},
		{namespace: "existing-single", seeded: true},
		{namespace: "existing-batch", seeded: true, batch: true},
	} {
		namespace := domainsequence.StaticNamespace(test.namespace)
		if test.seeded {
			err := state.ImportSequences(c.Context(), map[string]uint64{test.namespace: 7})
			c.Assert(err, tc.ErrorIsNil)
		}
		err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
			var err error
			if test.batch {
				_, err = NextNValues(ctx, s.state, tx, 3, namespace)
			} else {
				_, err = NextValue(ctx, s.state, tx, namespace)
			}
			if err != nil {
				return err
			}
			return rollback
		})
		c.Assert(err, tc.ErrorIs, rollback)

		stored, err := state.GetSequencesForExport(c.Context())
		c.Assert(err, tc.ErrorIsNil)
		value, exists := stored[test.namespace]
		c.Check(exists, tc.Equals, test.seeded)
		if test.seeded {
			c.Check(value, tc.Equals, uint64(7))
		}

		var next uint64
		err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
			var err error
			next, err = NextValue(ctx, s.state, tx, namespace)
			return err
		})
		c.Assert(err, tc.ErrorIsNil)
		if test.seeded {
			c.Check(next, tc.Equals, uint64(8))
		} else {
			c.Check(next, tc.Equals, uint64(0))
		}
	}
}

// TestSequenceConcurrentMixedAllocation checks that competing single and batch
// allocations never overlap or leave gaps in the committed sequence.
func (s *sequenceSuite) TestSequenceConcurrentMixedAllocation(c *tc.C) {
	const n = 20
	buf := make(chan []uint64, n)
	eg, ctx := errgroup.WithContext(c.Context())
	for i := range n {
		eg.Go(func() error {
			var values []uint64
			err := s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
				if i%2 == 0 {
					next, err := NextValue(ctx, s.state, tx, domainsequence.StaticNamespace("foo"))
					values = []uint64{next}
					return err
				}
				var err error
				values, err = NextNValues(ctx, s.state, tx, 3, domainsequence.StaticNamespace("foo"))
				return err
			})
			buf <- values
			return err
		})
	}
	c.Assert(eg.Wait(), tc.ErrorIsNil)
	close(buf)

	var values []uint64
	for batch := range buf {
		for i := 1; i < len(batch); i++ {
			c.Check(batch[i], tc.Equals, batch[i-1]+1)
		}
		values = append(values, batch...)
	}
	slices.Sort(values)
	c.Check(values, tc.HasLen, 2*n)
	for i, value := range values {
		c.Check(value, tc.Equals, uint64(i))
	}
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
