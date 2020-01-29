/*
* // Copyright 2020 Canonical Ltd.
* // Licensed under the AGPLv3, see LICENCE file for details.
 */

package spaces_test

import (
	"github.com/juju/juju/apiserver/facades/client/spaces"
	"github.com/juju/juju/apiserver/facades/client/spaces/mocks"
	gc "gopkg.in/check.v1"
)

type SpaceRenameSuite struct {
	mockOpFactory *mocks.MockOpFactory
	mockRenameOp  *mocks.MockRenameSpaceModelOp

	api *spaces.API
}

var _ = gc.Suite(&SpaceRenameSuite{})

func (s *SpaceRenameSuite) TearDownTest(c *gc.C) {
	s.api = nil
}
