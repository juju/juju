// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package snap

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/service/common"
)

type validationSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&validationSuite{})

func (*validationSuite) TestBackgroundServiceNeedsNonZeroName(c *gc.C) {
	empty := BackgroundService{}
	fail := empty.Validate()
	c.Check(fail, gc.ErrorMatches, "empty background service name not valid")
}

func (*validationSuite) TestBackgroundServiceNeedsLegalName(c *gc.C) {
	illegal := BackgroundService{Name: "23-==+++"}
	fail := illegal.Validate()
	c.Check(fail, gc.ErrorMatches, `background service name "23-==\+\+\+" not valid`)
}

func (*validationSuite) TestValidateJujuDbDaemon(c *gc.C) {
	service := BackgroundService{
		Name:            "daemon",
		EnableAtStartup: true,
	}
	err := service.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (*validationSuite) TestValidateJujuDbSnap(c *gc.C) {
	// manually
	services := []BackgroundService{{Name: "daemon"}}
	deps := []Installable{NewApp("core", "stable", "jailmode", nil, nil)}
	jujudb := NewApp("juju-db", "edge", "jailmode", services, deps)
	err := jujudb.Validate()
	c.Check(err, jc.ErrorIsNil)

	// via NewService
	jujudbService, err := NewService("juju-db", "", common.Conf{Desc: "juju-db snap"}, Command, "/path/to/config", "edge", "jailmode", []BackgroundService{}, []Installable{})
	c.Check(err, jc.ErrorIsNil)
	c.Check(jujudbService.Validate(), jc.ErrorIsNil)

}

type snapSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&snapSuite{})

func (*snapSuite) TestSnapCommandIsAValidCommand(c *gc.C) {
	_, err := exec.LookPath(Command)
	c.Check(err, gc.NotNil)
}

func (*snapSuite) TestConfigOverride(c *gc.C) {
	conf := common.Conf{
		Limit: map[string]string{
			"nofile": "64000",
		},
	}
	svc, err := NewService("juju-db", "", conf, Command, "/path/to/config", "latest", "strict", []BackgroundService{{
		Name: "daemon",
	}}, nil)
	c.Assert(err, jc.ErrorIsNil)

	dir := c.MkDir()

	s := &svc
	s.configDir = dir
	svc = *s

	err = svc.ConfigOverride()
	c.Assert(err, jc.ErrorIsNil)

	data, err := os.ReadFile(filepath.Join(dir, "snap.juju-db.daemon.service.d/overrides.conf"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, `
[Service]
LimitNOFILE=64000

`[1:])
}

type serviceSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&serviceSuite{})

func (*serviceSuite) TestInstall(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	clock := NewMockClock(ctrl)
	clock.EXPECT().Now().Return(time.Now()).AnyTimes()

	runnable := NewMockRunnable(ctrl)
	runnable.EXPECT().Execute("snap", []string{"install", "core"}).Return("", nil)
	runnable.EXPECT().Execute("snap", []string{"install", "--channel=9.9/stable", "juju-db"}).Return("", nil)

	conf := common.Conf{}
	prerequisites := []Installable{NewNamedApp("core")}
	backgroundServices := []BackgroundService{
		{
			Name:            "daemon",
			EnableAtStartup: true,
		},
	}
	service, err := NewService("juju-db", "juju-db", conf, Command, "/path/to/config", "9.9/stable", "", backgroundServices, prerequisites)
	c.Assert(err, jc.ErrorIsNil)

	s := &service
	s.runnable = runnable
	s.clock = clock
	service = *s

	err = service.Install()
	c.Assert(err, jc.ErrorIsNil)
}

func (*serviceSuite) TestInstallWithRetry(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	clock := NewMockClock(ctrl)
	clock.EXPECT().Now().Return(time.Now()).AnyTimes()
	clock.EXPECT().After(time.Second * 5).DoAndReturn(func(s time.Duration) <-chan time.Time {
		// Send the channel once we've been called, not before.
		ch := make(chan time.Time)
		go func() {
			ch <- time.Now().Add(time.Second * 5)
		}()
		return ch
	})

	runnable := NewMockRunnable(ctrl)
	runnable.EXPECT().Execute("snap", []string{"install", "core"}).Return("", errors.New("bad"))
	runnable.EXPECT().Execute("snap", []string{"install", "core"}).Return("", nil)
	runnable.EXPECT().Execute("snap", []string{"install", "--channel=9.9/stable", "juju-db"}).Return("", nil)

	conf := common.Conf{}
	prerequisites := []Installable{NewNamedApp("core")}
	backgroundServices := []BackgroundService{
		{
			Name:            "daemon",
			EnableAtStartup: true,
		},
	}
	service, err := NewService("juju-db", "juju-db", conf, Command, "/path/to/config", "9.9/stable", "", backgroundServices, prerequisites)
	c.Assert(err, jc.ErrorIsNil)

	s := &service
	s.runnable = runnable
	s.clock = clock
	service = *s

	err = service.Install()
	c.Assert(err, jc.ErrorIsNil)
}
