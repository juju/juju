// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"database/sql"
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/domain/schema/testing"
	jujutesting "github.com/juju/juju/internal/testing"
)

// ModelSuite is used to provide a sql.DB reference to tests.
// It is pre-populated with the model schema.
type ModelSuite struct {
	testing.ModelSuite

	watchableDB *TestWatchableDB
}

// SetUpTest is responsible for setting up a testing database suite initialised
// with the model schema.
func (s *ModelSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	s.watchableDB = NewTestWatchableDB(c, s.ModelUUID(), s.TxnRunner())

	// Prime the change stream, so that there is at least some
	// value in the stream, otherwise the changestream won't have any
	// bounds (terms) to work on.
	s.PrimeChangeStream(c)
}

func (s *ModelSuite) TearDownTest(c *tc.C) {
	if s.watchableDB != nil {
		// We could use workertest.DirtyKill here, but some workers are already
		// dead when we get here and it causes unwanted logs. This just ensures
		// that we don't have any addition workers running.
		killAndWait(c, s.watchableDB)
	}

	s.ModelSuite.TearDownTest(c)
}

// GetWatchableDB allows the ModelSuite to be a WatchableDBGetter
func (s *ModelSuite) GetWatchableDB(namespace string) (changestream.WatchableDB, error) {
	return s.watchableDB, nil
}

// AssertChangeStreamIdle returns if and when the change stream is idle.
// This is useful to ensure that the change stream is not processing any
// events before running a test.
func (s *ModelSuite) AssertChangeStreamIdle(c *tc.C) {
	timeout := time.After(jujutesting.LongWait)
	for {
		select {
		case state := <-s.watchableDB.states:
			if state == stateIdle {
				return
			}
		case <-timeout:
			c.Fatalf("timed out waiting for idle state")
		}
	}
}

// PrimeChangeStream the change stream with some initial data. This ensures
// that the change stream has some initial data otherwise the upper bound
// won't be set correctly. The model database has no triggers for the initial
// model, if this changes, we could remove the need for this.
// This is only for tests as we depend on the change stream to have at least
// some data, other wise we can't detect if the change stream is idle.
func (s *ModelSuite) PrimeChangeStream(c *tc.C) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO change_log_namespace (id, namespace, description) VALUES (666, 'test', 'all your bases are belong to us')
`); err != nil {
			return errors.Trace(err)
		}

		if _, err := tx.ExecContext(ctx, `
INSERT INTO change_log (edit_type_id, namespace_id, changed) VALUES (1, 666, 'foo')
`); err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
}
