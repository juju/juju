// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/mgo/v3/txn"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/client/spaces"
)

type SpaceRemoveSuite struct {
	space *spaces.MockRemoveSpace
}

var _ = gc.Suite(&SpaceRemoveSuite{})

func (s *SpaceRemoveSuite) TestSuccess(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	removeSpaceOps := []txn.Op{{
		C:      "1",
		Id:     "2",
		Remove: true,
	}, {
		C:      "1",
		Remove: false,
	}}

	s.space.EXPECT().RemoveSpaceOps().Return(removeSpaceOps, nil)

	op := spaces.NewRemoveSpaceOp(s.space)

	ops, err := op.Build(0)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ops, gc.HasLen, 2)
}

func (s *SpaceRemoveSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.space = spaces.NewMockRemoveSpace(ctrl)

	return ctrl
}
