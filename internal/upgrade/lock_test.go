// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrade

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/version"
)

type lockSuite struct {
	testing.IsolationSuite

	lock        *MockLock
	agent       *MockAgent
	agentConfig *MockConfig
}

var _ = gc.Suite(&lockSuite{})

func (s *lockSuite) TestNewLockSameVersionUnlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.agentConfig.EXPECT().UpgradedToVersion().Return(jujuversion.Current)
	c.Assert(NewLock(s.agentConfig, jujuversion.Current).IsUnlocked(), jc.IsTrue)
}

func (s *lockSuite) TestNewLockOldVersionLocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.agentConfig.EXPECT().UpgradedToVersion().Return(version.Number{})
	c.Assert(NewLock(s.agentConfig, jujuversion.Current).IsUnlocked(), jc.IsFalse)
}

func (s *lockSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.lock = NewMockLock(ctrl)
	s.agent = NewMockAgent(ctrl)
	s.agentConfig = NewMockConfig(ctrl)

	return ctrl
}
