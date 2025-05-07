// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"sync"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	coretesting "github.com/juju/juju/core/testing"
	"github.com/juju/juju/domain"
	schematesting "github.com/juju/juju/domain/schema/testing"
	domainsequence "github.com/juju/juju/domain/sequence"
)

type sequenceSuite struct {
	schematesting.ModelSuite

	state *domain.StateBase
}

var _ = tc.Suite(&sequenceSuite{})

func (s *sequenceSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)
	s.state = domain.NewStateBase(s.TxnRunnerFactory())
}

func (s *sequenceSuite) TestSequenceStaticNamespace(c *tc.C) {
	var next uint64
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		next, err = NextValue(ctx, s.state, tx, domainsequence.StaticNamespace("foo"))
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(next, tc.Equals, uint64(0))

	err = s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		next, err = NextValue(ctx, s.state, tx, domainsequence.StaticNamespace("foo"))
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(next, tc.Equals, uint64(1))
}

func (s *sequenceSuite) TestSequencePrefixNamespace(c *tc.C) {
	var next uint64
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		next, err = NextValue(ctx, s.state, tx, domainsequence.MakePrefixNamespace(domainsequence.StaticNamespace("foo"), "bar"))
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(next, tc.Equals, uint64(0))

	err = s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		next, err = NextValue(ctx, s.state, tx, domainsequence.MakePrefixNamespace(domainsequence.StaticNamespace("foo"), "bar"))
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(next, tc.Equals, uint64(1))
}

func (s *sequenceSuite) TestSequenceMultiple(c *tc.C) {
	got := sync.Map{}
	wg := sync.WaitGroup{}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			var next uint64
			err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
				var err error
				next, err = NextValue(ctx, s.state, tx, domainsequence.StaticNamespace("foo"))
				return err
			})
			c.Assert(err, tc.ErrorIsNil)
			got.Store(int(next), true)
		}()
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		wg.Wait()
	}()

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for goroutines to finish")
	}

	for i := 0; i < 100; i++ {
		_, ok := got.Load(i)
		c.Assert(ok, tc.IsTrue)
	}
}
