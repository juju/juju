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

func (s *controllerconfigSuite) TestControllerConfigRoundTrip(c *gc.C) {
	st := domainstate.NewState(testing.TxnRunnerFactory(s.TxnRunner()))
	srv := domainservice.NewService(st)

	controllerConfig := jujucontroller.Config{
		jujucontroller.AuditingEnabled:     true,
		jujucontroller.AuditLogCaptureArgs: false,
		jujucontroller.AuditLogMaxBackups:  100 * time.Millisecond,
		jujucontroller.PublicDNSAddress:    "controller.test.com:1234",
		jujucontroller.APIPortOpenDelay:    "100ms",
	}

	err := srv.UpdateControllerConfig(ctx.Background(), controllerConfig, nil)
	c.Assert(err, jc.ErrorIsNil)

	obtainedControllerConfig, err := srv.ControllerConfig(ctx.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(controllerConfig, jc.DeepEquals, obtainedControllerConfig)
}
