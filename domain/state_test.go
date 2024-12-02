// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	"context"
	"database/sql"
	"sync/atomic"
	"time"

	"github.com/canonical/sqlair"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/testing"
)

type stateSuite struct {
	schematesting.ControllerSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) TestStateBaseGetDB(c *gc.C) {
	f := s.TxnRunnerFactory()
	base := NewStateBase(f)
	db, err := base.DB()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(db, gc.NotNil)
}

func (s *stateSuite) TestStateBaseGetDBNilFactory(c *gc.C) {
	base := NewStateBase(nil)
	_, err := base.DB()
	c.Assert(err, gc.ErrorMatches, `nil getDB`)
}

func (s *stateSuite) TestStateBasePrepare(c *gc.C) {
	f := s.TxnRunnerFactory()
	base := NewStateBase(f)
	db, err := base.DB()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(db, gc.NotNil)

	// Prepare new query.
	stmt1, err := base.Prepare("SELECT name AS &M.* FROM sqlite_schema", sqlair.M{})
	c.Assert(err, jc.ErrorIsNil)
	// Validate prepared statement works as expected.
	var name any
	err = db.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		results := sqlair.M{}
		err := tx.Query(ctx, stmt1).Get(results)
		if err != nil {
			return err
		}
		name = results["name"]
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, "schema")

	// Retrieve previous statement.
	stmt2, err := base.Prepare("SELECT name AS &M.* FROM sqlite_schema", sqlair.M{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stmt1, gc.Equals, stmt2)
}

func (s *stateSuite) TestStateBasePrepareKeyClash(c *gc.C) {
	f := s.TxnRunnerFactory()
	base := NewStateBase(f)
	db, err := base.DB()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(db, gc.NotNil)

	// Prepare statement with TestType.
	{
		type TestType struct {
			WrongName string `db:"type"`
		}
		_, err := base.Prepare("SELECT &TestType.* FROM sqlite_schema", TestType{})
		c.Assert(err, jc.ErrorIsNil)
	}

	// Prepare statement with a different type of the same name, this will
	// retrieve the previously prepared statement which used the shadowed type.
	type TestType struct {
		Name string `db:"name"`
	}
	stmt, err := base.Prepare("SELECT &TestType.* FROM sqlite_schema", TestType{})
	c.Assert(err, jc.ErrorIsNil)

	// Try and run a query.
	err = db.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		var results TestType
		return tx.Query(ctx, stmt).Get(&results)
	})
	c.Assert(err, gc.ErrorMatches, `cannot get result: parameter with type "domain.TestType" missing, have type with same name: "domain.TestType"`)
}

func (s *stateSuite) TestStateBaseRunAtomicTransactionExists(c *gc.C) {
	f := s.TxnRunnerFactory()
	base := NewStateBase(f)
	db, err := base.DB()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(db, gc.NotNil)

	// Ensure that the transaction is sent via the AtomicContext.

	var tx *sqlair.TX
	err = base.RunAtomic(context.Background(), func(c AtomicContext) error {
		tx = c.(*atomicContext).tx
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(tx, gc.NotNil)
}

func (s *stateSuite) TestStateBaseRunAtomicPreventAtomicContextStoring(c *gc.C) {
	f := s.TxnRunnerFactory()
	base := NewStateBase(f)
	db, err := base.DB()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(db, gc.NotNil)

	// If the AtomicContext is stored outside of the transaction, it should
	// not be possible to use it to perform state changes, as the sqlair.TX
	// should be removed upon completion of the transaction.

	var txCtx AtomicContext
	err = base.RunAtomic(context.Background(), func(c AtomicContext) error {
		txCtx = c
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(txCtx, gc.NotNil)

	// Convert the AtomicContext to the underlying type.
	c.Check(txCtx.(*atomicContext).tx, gc.IsNil)
}

func (s *stateSuite) TestStateBaseRunAtomicContextValue(c *gc.C) {
	f := s.TxnRunnerFactory()
	base := NewStateBase(f)
	db, err := base.DB()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(db, gc.NotNil)

	// Ensure that the context is passed through to the AtomicContext.

	type contextKey string
	var key contextKey = "key"

	ctx := context.WithValue(context.Background(), key, "hello")

	var dbCtx AtomicContext
	err = base.RunAtomic(ctx, func(c AtomicContext) error {
		dbCtx = c
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(dbCtx, gc.NotNil)
	c.Check(dbCtx.Context().Value(key), gc.Equals, "hello")
}

func (s *stateSuite) TestStateBaseRunAtomicCancel(c *gc.C) {
	f := s.TxnRunnerFactory()
	base := NewStateBase(f)
	db, err := base.DB()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(db, gc.NotNil)

	// Make sure that the context symantics are respected in terms of
	// cancellation.

	ctx, cancel := context.WithCancel(context.Background())

	cancel()

	err = base.RunAtomic(ctx, func(dbCtx AtomicContext) error {
		c.Fatalf("should not be called")
		return err
	})
	c.Assert(err, jc.ErrorIs, context.Canceled)
}

func (s *stateSuite) TestStateBaseRunAtomicWithRun(c *gc.C) {
	f := s.TxnRunnerFactory()
	base := NewStateBase(f)
	db, err := base.DB()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(db, gc.NotNil)

	// Ensure that the Run method is called.

	var called bool
	err = base.RunAtomic(context.Background(), func(txCtx AtomicContext) error {
		return Run(txCtx, func(ctx context.Context, tx *sqlair.TX) error {
			called = true
			return nil
		})
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *stateSuite) TestStateBaseRunAtomicWithRunMultipleTimes(c *gc.C) {
	f := s.TxnRunnerFactory()
	base := NewStateBase(f)
	db, err := base.DB()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(db, gc.NotNil)

	// Ensure that the Run method is called.

	var called int
	err = base.RunAtomic(context.Background(), func(txCtx AtomicContext) error {
		for i := 0; i < 10; i++ {
			if err := Run(txCtx, func(ctx context.Context, tx *sqlair.TX) error {
				called++
				return nil
			}); err != nil {
				return err
			}
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, gc.Equals, 10)
}

func (s *stateSuite) TestStateBaseRunAtomicWithRunFailsConcurrently(c *gc.C) {
	f := s.TxnRunnerFactory()
	base := NewStateBase(f)
	db, err := base.DB()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(db, gc.NotNil)

	// Ensure that the run methods are correctly sequenced. Although there
	// is no guarantee on the order of execution after the first run. This
	// is undefined behaviour.

	var called int64
	err = base.RunAtomic(context.Background(), func(txCtx AtomicContext) error {
		firstErr := make(chan error)
		secondErr := make(chan error)

		start := make(chan struct{})
		go func() {
			err := Run(txCtx, func(ctx context.Context, tx *sqlair.TX) error {
				atomic.AddInt64(&called, 1)
				defer atomic.AddInt64(&called, 1)

				close(start)

				<-time.After(time.Millisecond * 100)

				return nil
			})
			firstErr <- err
		}()
		go func() {
			select {
			case <-start:
			case <-time.After(testing.LongWait):
				secondErr <- errors.Errorf("failed to start in time")
				return
			}

			err := Run(txCtx, func(ctx context.Context, tx *sqlair.TX) error {
				// If the first goroutine run is still running, the called
				// value will be 1. If it has completed, the called value
				// will be 2. This isn't exact, but it should be good enough
				// to ensure that the first run has completed.
				if atomic.LoadInt64(&called) != 2 {
					return errors.Errorf("called before first run completed")
				}

				atomic.AddInt64(&called, 1)

				return nil
			})
			secondErr <- err
		}()

		select {
		case err := <-firstErr:
			if err != nil {
				return err
			}
		case <-time.After(testing.LongWait):
			return errors.Errorf("failed to complete first run in time")
		}
		select {
		case err := <-secondErr:
			return err
		case <-time.After(testing.LongWait):
			return errors.Errorf("failed to complete second run in time")
		}
	})
	c.Assert(err, jc.ErrorIsNil)

	// Ensure that this is 3. 0 implies that it was never run, 1 implies that
	// the first run was never completed, 2 shows that the first run was
	// completed. Lastly 3 states that everything was run.
	c.Assert(called, gc.Equals, int64(3))
}

func (s *stateSuite) TestStateBaseRunAtomicWithRunPreparedStatements(c *gc.C) {
	f := s.TxnRunnerFactory()
	base := NewStateBase(f)
	db, err := base.DB()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(db, gc.NotNil)

	// Ensure that the Run method can use sqlair prepared statements.

	type N struct {
		Name string `db:"name"`
	}

	stmt, err := base.Prepare("SELECT &N.* FROM sqlite_schema WHERE name='schema'", N{})
	c.Assert(err, jc.ErrorIsNil)

	var result []N
	err = base.RunAtomic(context.Background(), func(txCtx AtomicContext) error {
		return Run(txCtx, func(ctx context.Context, tx *sqlair.TX) error {
			return tx.Query(ctx, stmt).GetAll(&result)
		})
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Check(result[0].Name, gc.Equals, "schema")
}

func (s *stateSuite) TestStateBaseRunAtomicWithRunDoesNotLeakError(c *gc.C) {
	f := s.TxnRunnerFactory()
	base := NewStateBase(f)
	db, err := base.DB()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(db, gc.NotNil)

	// Ensure that the Run method does not leak sql.ErrNoRows.

	type N struct {
		Name string `db:"name"`
	}

	stmt, err := base.Prepare("SELECT &N.* FROM sqlite_schema WHERE name='something something something'", N{})
	c.Assert(err, jc.ErrorIsNil)

	var result N
	err = base.RunAtomic(context.Background(), func(txCtx AtomicContext) error {
		return Run(txCtx, func(ctx context.Context, tx *sqlair.TX) error {
			return tx.Query(ctx, stmt).Get(&result)
		})
	})
	c.Assert(err, gc.Not(jc.ErrorIs), sql.ErrNoRows)
	c.Assert(err, gc.Not(jc.ErrorIs), sqlair.ErrNoRows)
}
