// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

import (
	"errors"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/service"
	"github.com/juju/juju/internal/service/common"
	"github.com/juju/juju/internal/service/mocks"
	"github.com/juju/juju/internal/service/systemd"
)

type serviceSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) TestNewService(c *gc.C) {
	cfg := common.Conf{Desc: "test", ExecStart: "/path/to/script"}
	svc, err := service.NewService("fred", cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(svc, gc.FitsTypeOf, &systemd.Service{})
	c.Assert(svc.Name(), gc.Equals, "fred")
	c.Assert(svc.Conf(), jc.DeepEquals, cfg)
}

func (s *serviceSuite) TestNewServiceMissingName(c *gc.C) {
	_, err := service.NewService("", common.Conf{})
	c.Assert(err, gc.ErrorMatches, `.*missing name.*`)
}

func (s *serviceSuite) TestListServices(c *gc.C) {
	_, err := service.ListServices()
	c.Assert(err, jc.ErrorIsNil)
}

func (*serviceSuite) TestListServicesScript(c *gc.C) {
	script := service.ListServicesScript()
	expected := `/bin/systemctl list-unit-files --no-legend --no-page -l -t service` +
		` | grep -o -P '^\w[\S]*(?=\.service)'`
	c.Assert(script, gc.Equals, expected)
}

func (s *serviceSuite) TestInstallAndStartOkay(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	svc := mocks.NewMockService(ctrl)
	svc.EXPECT().Name().Return("fred")
	svc.EXPECT().Install().Return(nil)
	svc.EXPECT().Stop().Return(nil)
	svc.EXPECT().Start().Return(nil)

	err := service.InstallAndStart(svc)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestInstallAndStartRetry(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	svc := mocks.NewMockService(ctrl)
	svc.EXPECT().Name().Return("fred")
	svc.EXPECT().Install().Return(nil)
	svc.EXPECT().Stop().Return(errors.New("stop error"))
	svc.EXPECT().Start().Return(errors.New("start error"))
	svc.EXPECT().Stop().Return(nil)
	svc.EXPECT().Start().Return(nil)

	err := service.InstallAndStart(svc)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestInstallAndStartFail(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	svc := mocks.NewMockService(ctrl)
	svc.EXPECT().Name().Return("fred")
	svc.EXPECT().Install().Return(nil)
	for i := 0; i < 4; i++ {
		svc.EXPECT().Stop().Return(nil)
		svc.EXPECT().Start().Return(errors.New("start error"))
	}

	err := service.InstallAndStart(svc)
	c.Assert(err, gc.ErrorMatches, "start error")
}

type restartSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&restartSuite{})

func (s *restartSuite) TestRestartStopAndStart(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	svc := mocks.NewMockService(ctrl)
	svc.EXPECT().Stop().Return(nil)
	svc.EXPECT().Start().Return(nil)

	s.PatchValue(&service.NewService, func(name string, conf common.Conf) (service.Service, error) {
		c.Assert(name, gc.Equals, "fred")
		return svc, nil
	})
	err := service.Restart("fred")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *restartSuite) TestRestartFailStop(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	svc := mocks.NewMockService(ctrl)
	svc.EXPECT().Stop().Return(errors.New("boom"))
	svc.EXPECT().Start().Return(nil)

	s.PatchValue(&service.NewService, func(name string, conf common.Conf) (service.Service, error) {
		c.Assert(name, gc.Equals, "fred")
		return svc, nil
	})
	err := service.Restart("fred")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *restartSuite) TestRestartFailStart(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	svc := mocks.NewMockService(ctrl)
	svc.EXPECT().Stop().Return(nil)
	svc.EXPECT().Start().Return(errors.New("boom"))

	s.PatchValue(&service.NewService, func(name string, conf common.Conf) (service.Service, error) {
		c.Assert(name, gc.Equals, "fred")
		return svc, nil
	})
	err := service.Restart("fred")
	c.Assert(err, gc.ErrorMatches, `failed to restart service "fred": boom`)
}
