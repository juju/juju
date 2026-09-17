// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn/v3"
	gc "gopkg.in/check.v1"
)

type cleanupInternalSuite struct {
	internalStateSuite
}

var _ = gc.Suite(&cleanupInternalSuite{})

func (s *cleanupInternalSuite) TestRemoveRemoteApplicationsForDyingModelContinuesAfterFailure(c *gc.C) {
	s.assertRemoveRemoteApplicationsAfterFailure(c, true)
}

func (s *cleanupInternalSuite) TestRemoveRemoteApplicationsForDyingModelStopsAfterFailure(c *gc.C) {
	s.assertRemoveRemoteApplicationsAfterFailure(c, false)
}

func (s *cleanupInternalSuite) assertRemoveRemoteApplicationsAfterFailure(c *gc.C, force bool) {
	for _, name := range []string{"first", "second"} {
		_, err := s.state.AddRemoteApplication(AddRemoteApplicationParams{
			Name:        name,
			SourceModel: names.NewModelTag("source-model"),
		})
		c.Assert(err, jc.ErrorIsNil)
	}
	// Fail the first removal regardless of which application is read first.
	failure := errors.New("transaction failed")
	s.PatchValue(&s.state.database, &cleanupFailureDatabase{
		Database: s.state.database,
		failure:  failure,
	})
	err := s.state.removeRemoteApplicationsForDyingModel(DestroyModelParams{Force: &force})
	c.Check(err, jc.ErrorIs, failure)
	remaining, err := s.state.AllRemoteApplications()
	c.Assert(err, jc.ErrorIsNil)
	wantRemaining := 2
	if force {
		wantRemaining = 1
	}
	c.Check(remaining, gc.HasLen, wantRemaining)

	// The failed application can be removed on the next pass.
	c.Assert(s.state.removeRemoteApplicationsForDyingModel(DestroyModelParams{Force: &force}), jc.ErrorIsNil)
	remaining, err = s.state.AllRemoteApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(remaining, gc.HasLen, 0)
}

type cleanupFailureDatabase struct {
	Database
	failure error
}

func (db *cleanupFailureDatabase) Run(source jujutxn.TransactionSource) error {
	if db.failure != nil {
		err := db.failure
		db.failure = nil
		return err
	}
	return db.Database.Run(source)
}
