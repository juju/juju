// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sequence

import (
	"context"
	"sync"
	"time"

	"github.com/canonical/sqlair"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/core/testing"
	"github.com/juju/juju/domain"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type sequenceSuite struct {
	schematesting.ModelSuite

	state *domain.StateBase
}

var _ = gc.Suite(&sequenceSuite{})

func (s *sequenceSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)
	s.state = domain.NewStateBase(s.TxnRunnerFactory())
}

func (s *sequenceSuite) TestSequence(c *gc.C) {
	var next uint64
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		next, err = NextValue(ctx, s.state, tx, "foo")
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(next, gc.Equals, uint64(0))

	err = s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		next, err = NextValue(ctx, s.state, tx, "foo")
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(next, gc.Equals, uint64(1))
}

func (s *sequenceSuite) TestSequenceMultiple(c *gc.C) {
	got := sync.Map{}
	wg := sync.WaitGroup{}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			var next uint64
			err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
				var err error
				next, err = NextValue(ctx, s.state, tx, "foo")
				return err
			})
			c.Assert(err, jc.ErrorIsNil)
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
		c.Assert(ok, jc.IsTrue)
	}
}
