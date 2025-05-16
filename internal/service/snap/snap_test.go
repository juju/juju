// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package snap

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/service/common"
	"github.com/juju/juju/internal/testhelpers"
)

type validationSuite struct {
	testhelpers.IsolationSuite
}

func TestValidationSuite(t *stdtesting.T) { tc.Run(t, &validationSuite{}) }
func (*validationSuite) TestBackgroundServiceNeedsNonZeroName(c *tc.C) {
	empty := BackgroundService{}
	fail := empty.Validate()
	c.Check(fail, tc.ErrorMatches, "empty background service name not valid")
}

func (*validationSuite) TestBackgroundServiceNeedsLegalName(c *tc.C) {
	illegal := BackgroundService{Name: "23-==+++"}
	fail := illegal.Validate()
	c.Check(fail, tc.ErrorMatches, `background service name "23-==\+\+\+" not valid`)
}

func (*validationSuite) TestValidateJujuDbDaemon(c *tc.C) {
	service := BackgroundService{
		Name:            "daemon",
		EnableAtStartup: true,
	}
	err := service.Validate()

	c.Check(err, tc.ErrorIsNil)
}

func (*validationSuite) TestValidateJujuDbSnap(c *tc.C) {
	// manually
	services := []BackgroundService{{Name: "daemon"}}
	deps := []Installable{&App{
		name:              "core",
		channel:           "stable",
		confinementPolicy: "jailmode",
	}}

	jujudb := &App{
		name:               "juju-db",
		channel:            "edge",
		confinementPolicy:  "jailmode",
		backgroundServices: services,
		prerequisites:      deps,
	}
	err := jujudb.Validate()
	c.Check(err, tc.ErrorIsNil)

	// via NewService
	jujudbService, err := NewService(ServiceConfig{
		ServiceName:       "juju-db",
		Conf:              common.Conf{Desc: "juju-db snap"},
		SnapExecutable:    Command,
		ConfigDir:         "/path/to/config",
		Channel:           "edge",
		ConfinementPolicy: "jailmode",
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(jujudbService.Validate(), tc.ErrorIsNil)
}

func (*validationSuite) TestValidateLocalSnap(c *tc.C) {
	dir := c.MkDir()
	snapPath := filepath.Join(dir, "juju-db_123.snap")
	assertPath := filepath.Join(dir, "juju-db_123.assert")

	f, err := os.Create(snapPath)
	c.Assert(err, tc.ErrorIsNil)
	f.Close()

	f, err = os.Create(assertPath)
	c.Assert(err, tc.ErrorIsNil)
	f.Close()

	// manually
	jujudb := &App{
		name:        "juju-db",
		path:        snapPath,
		assertsPath: assertPath,
	}
	err = jujudb.Validate()
	c.Check(err, tc.ErrorIsNil)

	// via NewService
	jujudbService, err := NewService(ServiceConfig{
		ServiceName:     "juju-db",
		SnapPath:        snapPath,
		SnapAssertsPath: assertPath,
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(jujudbService.Validate(), tc.ErrorIsNil)
}

type snapSuite struct {
	testhelpers.IsolationSuite
}

func TestSnapSuite(t *stdtesting.T) { tc.Run(t, &snapSuite{}) }
func (*snapSuite) TestSnapCommandIsAValidCommand(c *tc.C) {
	_, err := exec.LookPath(Command)
	c.Check(err, tc.NotNil)
}

func (*snapSuite) TestConfigOverride(c *tc.C) {
	conf := common.Conf{
		Limit: map[string]string{
			"nofile": "64000",
		},
	}
	svc, err := NewService(ServiceConfig{
		ServiceName:       "juju-db",
		Conf:              conf,
		SnapExecutable:    Command,
		ConfigDir:         "/path/to/config",
		Channel:           "latest",
		ConfinementPolicy: "strict",
		BackgroundServices: []BackgroundService{{
			Name: "daemon",
		}},
	})
	c.Assert(err, tc.ErrorIsNil)

	dir := c.MkDir()

	s := &svc
	s.configDir = dir
	svc = *s

	err = svc.ConfigOverride()
	c.Assert(err, tc.ErrorIsNil)

	data, err := os.ReadFile(filepath.Join(dir, "snap.juju-db.daemon.service.d/overrides.conf"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(data), tc.Equals, `
[Service]
LimitNOFILE=64000

`[1:])
}

type serviceSuite struct {
	testhelpers.IsolationSuite
}

func TestServiceSuite(t *stdtesting.T) { tc.Run(t, &serviceSuite{}) }
func (*serviceSuite) TestInstall(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	clock := NewMockClock(ctrl)
	clock.EXPECT().Now().Return(time.Now()).AnyTimes()

	runnable := NewMockRunnable(ctrl)
	runnable.EXPECT().Execute("snap", []string{"install", "core"}).Return("", nil)
	runnable.EXPECT().Execute("snap", []string{"install", "--channel=9.9/stable", "juju-db"}).Return("", nil)

	conf := common.Conf{}
	prerequisites := []Installable{&App{name: "core"}}
	backgroundServices := []BackgroundService{
		{
			Name:            "daemon",
			EnableAtStartup: true,
		},
	}
	service, err := NewService(ServiceConfig{
		ServiceName:        "juju-db",
		Conf:               conf,
		SnapExecutable:     Command,
		ConfigDir:          "/path/to/config",
		Channel:            "9.9/stable",
		BackgroundServices: backgroundServices,
		Prerequisites:      prerequisites,
	})
	c.Assert(err, tc.ErrorIsNil)

	s := &service
	s.runnable = runnable
	s.clock = clock
	service = *s

	err = service.Install()
	c.Assert(err, tc.ErrorIsNil)
}

func (*serviceSuite) TestInstallWithRetry(c *tc.C) {
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
	prerequisites := []Installable{&App{name: "core"}}
	backgroundServices := []BackgroundService{
		{
			Name:            "daemon",
			EnableAtStartup: true,
		},
	}
	service, err := NewService(ServiceConfig{
		ServiceName:        "juju-db",
		Conf:               conf,
		SnapExecutable:     Command,
		ConfigDir:          "/path/to/config",
		Channel:            "9.9/stable",
		BackgroundServices: backgroundServices,
		Prerequisites:      prerequisites,
	})
	c.Assert(err, tc.ErrorIsNil)

	s := &service
	s.runnable = runnable
	s.clock = clock
	service = *s

	err = service.Install()
	c.Assert(err, tc.ErrorIsNil)
}

func (*serviceSuite) TestInstallLocalSnapWithRetry(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	dir := c.MkDir()
	snapPath := filepath.Join(dir, "juju-db_123.snap")
	assertPath := filepath.Join(dir, "juju-db_123.assert")

	f, err := os.Create(snapPath)
	c.Assert(err, tc.ErrorIsNil)
	f.Close()

	f, err = os.Create(assertPath)
	c.Assert(err, tc.ErrorIsNil)
	f.Close()

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
	runnable.EXPECT().Execute("snap", []string{"ack", assertPath}).Return("", errors.New("bad"))
	runnable.EXPECT().Execute("snap", []string{"ack", assertPath}).Return("", nil)
	runnable.EXPECT().Execute("snap", []string{"install", snapPath}).Return("", nil)

	service, err := NewService(ServiceConfig{
		ServiceName:     "juju-db",
		SnapPath:        snapPath,
		SnapAssertsPath: assertPath,
		SnapExecutable:  Command,
		Channel:         "9.9/stable",
	})
	c.Assert(err, tc.ErrorIsNil)

	s := &service
	s.runnable = runnable
	s.clock = clock
	service = *s

	err = service.Install()
	c.Assert(err, tc.ErrorIsNil)
}
