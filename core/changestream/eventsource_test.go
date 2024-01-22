// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestream

import (
	"github.com/juju/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/database"
)

type changestreamSuite struct {
	testing.IsolationSuite

	txnRunner *MockTxnRunner
}

var _ = gc.Suite(&changestreamSuite{})

func (s *changestreamSuite) TestTxnRunnerFactory(c *gc.C) {
	db, err := NewTxnRunnerFactory(s.getWatchableDB)()
	c.Assert(err, gc.IsNil)
	c.Assert(db, gc.NotNil)
}

func (s *changestreamSuite) TestTxnRunnerFactoryForNamespace(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Test multiple function return signatures to verify the generic behaviour.
	db, err := database.NewTxnRunnerFactoryForNamespace(func(string) (database.TxnRunner, error) {
		return s.txnRunner, nil
	}, "any-old-namespace")()
	c.Assert(err, gc.IsNil)
	c.Assert(db, gc.NotNil)

	db, err = database.NewTxnRunnerFactoryForNamespace(s.getWatchableDBForNameSpace, "any-old-namespace")()
	c.Assert(err, gc.IsNil)
	c.Assert(db, gc.NotNil)
}

func (s *changestreamSuite) getWatchableDB() (WatchableDB, error) {
	return &stubWatchableDB{TxnRunner: s.txnRunner}, nil
}

func (s *changestreamSuite) getWatchableDBForNameSpace(_ string) (WatchableDB, error) {
	return &stubWatchableDB{TxnRunner: s.txnRunner}, nil
}

func (s *changestreamSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.txnRunner = NewMockTxnRunner(ctrl)

	return ctrl
}

type stubWatchableDB struct {
	database.TxnRunner
	EventSource
	EventWatcher
}
