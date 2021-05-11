// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasupgraderembedded_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v2/arch"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	coreos "github.com/juju/juju/core/os"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker/caasupgraderembedded"
	"github.com/juju/juju/worker/caasupgraderembedded/mocks"
	"github.com/juju/juju/worker/gate"
)

type workerSuite struct {
	coretesting.BaseSuite

	agentTag names.Tag
	config   caasupgraderembedded.Config

	confVersion    version.Number
	upgraderClient *mocks.MockUpgraderClient

	upgradeStepsComplete gate.Lock
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.upgradeStepsComplete = gate.NewLock()
	s.agentTag = names.NewUnitTag("snappass/0")
}

func (s *workerSuite) patchVersion(v version.Binary) {
	s.PatchValue(&arch.HostArch, func() string { return v.Arch })
	s.PatchValue(&coreos.HostOS, func() coreos.OSType { return coreos.Ubuntu })
	s.PatchValue(&jujuversion.Current, v.Number)
}

func (s *workerSuite) initConfig(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	var err error
	s.confVersion, err = version.Parse("2.9.0")
	c.Assert(err, jc.ErrorIsNil)

	s.upgraderClient = mocks.NewMockUpgraderClient(ctrl)

	s.config = caasupgraderembedded.Config{
		UpgraderClient:     s.upgraderClient,
		AgentTag:           s.agentTag,
		UpgradeStepsWaiter: s.upgradeStepsComplete,
		Logger:             loggo.GetLogger("test"),
	}
	return ctrl
}

func (s *workerSuite) TestValidateConfig(c *gc.C) {
	_ = s.initConfig(c)

	s.testValidateConfig(c, func(config *caasupgraderembedded.Config) {
		config.UpgraderClient = nil
	}, `missing UpgraderClient not valid`)

	s.testValidateConfig(c, func(config *caasupgraderembedded.Config) {
		config.Logger = nil
	}, `missing Logger not valid`)

	s.testValidateConfig(c, func(config *caasupgraderembedded.Config) {
		config.AgentTag = names.NewApplicationTag("snappass")
	}, `tag "application-snappass" not valid`)

}

func (s *workerSuite) testValidateConfig(c *gc.C, f func(*caasupgraderembedded.Config), expect string) {
	config := s.config
	f(&config)
	c.Check(config.Validate(), gc.ErrorMatches, expect)
}

func (s *workerSuite) TestStartStop(c *gc.C) {
	ctrl := s.initConfig(c)
	defer ctrl.Finish()

	s.patchVersion(
		version.Binary{
			Number: s.confVersion,
			Arch:   "amd64",
		},
	)
	s.upgraderClient.EXPECT().SetVersion(s.agentTag.String(), caasupgraderembedded.ToBinaryVersion(s.confVersion, "ubuntu")).Return(nil)

	w, err := caasupgraderembedded.NewUpgrader(s.config)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, w)
}
