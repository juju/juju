// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllerconfig

import (
	ctx "context"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/database/testing"
	domainservice "github.com/juju/juju/domain/controllerconfig/service"
	domainstate "github.com/juju/juju/domain/controllerconfig/state"
)

type controllerconfigSuite struct {
	testing.ControllerSuite
}

var _ = gc.Suite(&controllerconfigSuite{})

func (s *controllerconfigSuite) TestControllerConfigRoundTrips(c *gc.C) {
	st := domainstate.NewState(testing.TxnRunnerFactory(s.TxnRunner()))
	srv := domainservice.NewService(st)

	cfgIn := jujucontroller.Config{
		jujucontroller.AuditingEnabled:     true,
		jujucontroller.AuditLogCaptureArgs: false,
		jujucontroller.AuditLogMaxBackups:  10,
		jujucontroller.PublicDNSAddress:    "controller.test.com:1234",
		jujucontroller.APIPortOpenDelay:    "100ms",
	}

	err := srv.UpdateControllerConfig(ctx.Background(), cfgIn, nil)
	c.Assert(err, jc.ErrorIsNil)

	cfgOut, err := srv.ControllerConfig(ctx.Background())
	c.Assert(err, jc.ErrorIsNil)

	err = srv.UpdateControllerConfig(ctx.Background(), cfgOut, nil)
	c.Assert(err, jc.ErrorIsNil)

	cfgOut, err = srv.ControllerConfig(ctx.Background())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(cfgOut.AuditingEnabled(), jc.IsTrue)
	c.Assert(cfgOut.AuditLogCaptureArgs(), jc.IsFalse)
	c.Assert(cfgOut.AuditLogMaxBackups(), gc.Equals, 10)
	c.Assert(cfgOut.PublicDNSAddress(), gc.Equals, "controller.test.com:1234")
	c.Assert(cfgOut.APIPortOpenDelay(), gc.Equals, 100*time.Millisecond)
}
