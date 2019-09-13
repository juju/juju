// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelgeneration_test

import (
	"time"

	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/apiserver/facades/client/modelgeneration"
	"github.com/juju/juju/apiserver/facades/client/modelgeneration/mocks"
	"github.com/juju/juju/core/settings"
	"github.com/juju/juju/testing"
)

type commitBranchSuite struct {
	testing.BaseSuite

	st       *mocks.MockCommitBranchState
	br       *mocks.MockCommitBranchGen
	app      *mocks.MockCommitBranchApp
	settings *mocks.MockSettings
}

var _ = gc.Suite(&commitBranchSuite{})

func (s *commitBranchSuite) TestSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()
	settingsChanges := settings.ItemChanges{settings.MakeAddition("key", "new-value")}
	assigned := map[string][]string{"app": {"app/0"}}

	brExp := s.br.EXPECT()
	brExp.ValidateForCompletion().Return(nil)
	brExp.AssignedUnits().Return(assigned)
	brExp.Config().Return(map[string]settings.ItemChanges{"app": settingsChanges})
	brExp.CompleteOps(assigned, &now, "test-user").Return([]txn.Op{{}}, 666, nil)

	stExp := s.st.EXPECT()
	stExp.Application("app").Return(s.app, nil)
	stExp.ControllerTimestamp().Return(&now, nil)

	s.app.EXPECT().UnitNames().Return([]string{"app/0", "app/1"}, nil)
	s.app.EXPECT().CharmConfigKey().Return("app#settings")

	s.settings.EXPECT().DeltaOps("app#settings", settingsChanges).Return([]txn.Op{{}}, nil)

	op := modelgeneration.NewCommitBranchModelOp(s.st, s.br, "test-user", s.settings)

	ops, err := op.Build(0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ops, gc.HasLen, 2)
	c.Assert(op.GetModelGen(), gc.Equals, 666)
}

func (s *commitBranchSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = mocks.NewMockCommitBranchState(ctrl)
	s.br = mocks.NewMockCommitBranchGen(ctrl)
	s.app = mocks.NewMockCommitBranchApp(ctrl)
	s.settings = mocks.NewMockSettings(ctrl)

	return ctrl
}
