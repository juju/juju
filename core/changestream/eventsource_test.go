// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestream

import (
	"context"
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/internal/testhelpers"
)

type changestreamSuite struct {
	testhelpers.IsolationSuite

	txnRunner *MockTxnRunner
}

func TestChangestreamSuite(t *testing.T) {
	tc.Run(t, &changestreamSuite{})
}

func (s *changestreamSuite) TestTxnRunnerFactory(c *tc.C) {
	db, err := NewTxnRunnerFactory(s.getWatchableDB)(c.Context())
	c.Assert(err, tc.IsNil)
	c.Assert(db, tc.NotNil)
}

func (s *changestreamSuite) TestTxnRunnerFactoryForNamespace(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Test multiple function return signatures to verify the generic behaviour.
	db, err := database.NewTxnRunnerFactoryForNamespace(func(context.Context, string) (database.TxnRunner, error) {
		return s.txnRunner, nil
	}, "any-old-namespace")(c.Context())
	c.Assert(err, tc.IsNil)
	c.Assert(db, tc.NotNil)

	db, err = database.NewTxnRunnerFactoryForNamespace(s.getWatchableDBForNameSpace, "any-old-namespace")(c.Context())
	c.Assert(err, tc.IsNil)
	c.Assert(db, tc.NotNil)
}

func (s *changestreamSuite) getWatchableDB(context.Context) (WatchableDB, error) {
	return &stubWatchableDB{TxnRunner: s.txnRunner}, nil
}

func (s *changestreamSuite) getWatchableDBForNameSpace(context.Context, string) (WatchableDB, error) {
	return &stubWatchableDB{TxnRunner: s.txnRunner}, nil
}

func (s *changestreamSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.txnRunner = NewMockTxnRunner(ctrl)

	return ctrl
}

type stubWatchableDB struct {
	database.TxnRunner
	EventSource
}
