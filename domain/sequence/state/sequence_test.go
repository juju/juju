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
