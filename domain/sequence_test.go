// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	"context"
	"sync"
	
	"github.com/canonical/sqlair"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type sequenceSuite struct {
	schematesting.ModelSuite

	state *StateBase
}

var _ = gc.Suite(&sequenceSuite{})

func (s *sequenceSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)
	s.state = NewStateBase(s.TxnRunnerFactory())
}

func (s *sequenceSuite) TestSequence(c *gc.C) {
	var next uint
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		next, err = NextSequenceValue(ctx, s.state, tx, "foo")
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(next, gc.Equals, uint(0))
	
	err = s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		next, err = NextSequenceValue(ctx, s.state, tx, "foo")
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(next, gc.Equals, uint(1))
}

func (s *sequenceSuite) TestSequenceMultiple(c *gc.C) {
	got := sync.Map{}
	wg :=  sync.WaitGroup{}
	for  i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			var next uint
			err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
				var err error
				next, err = NextSequenceValue(ctx, s.state, tx, "foo")
				return err
			})
			c.Assert(err, jc.ErrorIsNil)
			got.Store(int(next), true)
		}()
	}
	wg.Wait()
	for i := 0; i < 100; i++ {
		_, ok := got.Load(i)
		c.Assert(ok, jc.IsTrue)
	}
}
