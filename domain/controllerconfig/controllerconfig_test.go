// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllerconfig

import (
	ctx "context"
	"time"

	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/domain/controllerconfig/bootstrap"
	"github.com/juju/juju/domain/controllerconfig/service"
	domainstate "github.com/juju/juju/domain/controllerconfig/state"
	schematesting "github.com/juju/juju/domain/schema/testing"
	jujutesting "github.com/juju/juju/testing"
)

type controllerconfigSuite struct {
	schematesting.ControllerSuite
}

var _ = gc.Suite(&controllerconfigSuite{})

func (s *controllerconfigSuite) TestControllerConfigRoundTrips(c *gc.C) {
	st := domainstate.NewState(s.TxnRunnerFactory())
	srv := service.NewService(st)

	cfgMap := map[string]any{
		controller.AuditingEnabled:        true,
		controller.AuditLogCaptureArgs:    false,
		controller.AuditLogMaxBackups:     10,
		controller.PublicDNSAddress:       "controller.test.com:1234",
		controller.APIPortOpenDelay:       "100ms",
		controller.MigrationMinionWaitMax: "101ms",
		controller.PruneTxnSleepTime:      "102ms",
		controller.QueryTracingThreshold:  "103ms",
		controller.MaxDebugLogDuration:    "104ms",
	}
	cfgIn, err := controller.NewConfig(
		jujutesting.ControllerTag.Id(),
		jujutesting.CACert,
		cfgMap,
	)
	c.Assert(err, jc.ErrorIsNil)

	err = bootstrap.InsertInitialControllerConfig(cfgIn)(ctx.Background(), s.TxnRunner(), s.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	cfgOut, err := srv.ControllerConfig(ctx.Background())
	c.Assert(err, jc.ErrorIsNil)

	selected := filterConfig(cfgOut)

	err = srv.UpdateControllerConfig(ctx.Background(), selected, nil)
	c.Assert(err, jc.ErrorIsNil)

	cfgOut, err = srv.ControllerConfig(ctx.Background())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(cfgOut.AuditingEnabled(), jc.IsTrue)
	c.Assert(cfgOut.AuditLogCaptureArgs(), jc.IsFalse)
	c.Assert(cfgOut.AuditLogMaxBackups(), gc.Equals, 10)
	c.Assert(cfgOut.PublicDNSAddress(), gc.Equals, "controller.test.com:1234")
	c.Assert(cfgOut.APIPortOpenDelay(), gc.Equals, 100*time.Millisecond)
	c.Assert(cfgOut.MigrationMinionWaitMax(), gc.Equals, 101*time.Millisecond)
	c.Assert(cfgOut.PruneTxnSleepTime(), gc.Equals, 102*time.Millisecond)
	c.Assert(cfgOut.QueryTracingThreshold(), gc.Equals, 103*time.Millisecond)
	c.Assert(cfgOut.MaxDebugLogDuration(), gc.Equals, 104*time.Millisecond)
}

func keys(m map[string]any) set.Strings {
	var result []string
	for k := range m {
		result = append(result, k)
	}
	return set.NewStrings(result...)
}

func filterConfig(m map[string]any) map[string]any {
	k := keys(m).Difference(controller.AllowedUpdateConfigAttributes)
	for _, key := range k.Values() {
		delete(m, key)
	}
	return m
}
