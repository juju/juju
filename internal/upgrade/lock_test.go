// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrade

import (
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/testhelpers"
)

type lockSuite struct {
	testhelpers.IsolationSuite

	lock        *MockLock
	agent       *MockAgent
	agentConfig *MockConfig
}

var _ = tc.Suite(&lockSuite{})

func (s *lockSuite) TestNewLockSameVersionUnlocked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.agentConfig.EXPECT().UpgradedToVersion().Return(jujuversion.Current)
	c.Assert(NewLock(s.agentConfig, jujuversion.Current).IsUnlocked(), tc.IsTrue)
}

func (s *lockSuite) TestNewLockOldVersionLocked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.agentConfig.EXPECT().UpgradedToVersion().Return(semversion.Number{})
	c.Assert(NewLock(s.agentConfig, jujuversion.Current).IsUnlocked(), tc.IsFalse)
}

func (s *lockSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.lock = NewMockLock(ctrl)
	s.agent = NewMockAgent(ctrl)
	s.agentConfig = NewMockConfig(ctrl)

	return ctrl
}
