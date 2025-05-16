// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllerconfig

import (
	stdtesting "testing"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/tc"

	"github.com/juju/juju/controller"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/controllerconfig/bootstrap"
	"github.com/juju/juju/domain/controllerconfig/service"
	domainstate "github.com/juju/juju/domain/controllerconfig/state"
	schematesting "github.com/juju/juju/domain/schema/testing"
	jujutesting "github.com/juju/juju/internal/testing"
)

type controllerconfigSuite struct {
	schematesting.ControllerSuite
}

func TestControllerconfigSuite(t *stdtesting.T) { tc.Run(t, &controllerconfigSuite{}) }
func (s *controllerconfigSuite) TestControllerConfigRoundTrips(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)

	controllerModelUUID := coremodel.UUID(jujutesting.ModelTag.Id())

	err = bootstrap.InsertInitialControllerConfig(cfgIn, controllerModelUUID)(c.Context(), s.TxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	cfgOut, err := srv.ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	selected := filterConfig(cfgOut)

	err = srv.UpdateControllerConfig(c.Context(), selected, nil)
	c.Assert(err, tc.ErrorIsNil)

	cfgOut, err = srv.ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(cfgOut.AuditingEnabled(), tc.IsTrue)
	c.Assert(cfgOut.AuditLogCaptureArgs(), tc.IsFalse)
	c.Assert(cfgOut.AuditLogMaxBackups(), tc.Equals, 10)
	c.Assert(cfgOut.PublicDNSAddress(), tc.Equals, "controller.test.com:1234")
	c.Assert(cfgOut.APIPortOpenDelay(), tc.Equals, 100*time.Millisecond)
	c.Assert(cfgOut.MigrationMinionWaitMax(), tc.Equals, 101*time.Millisecond)
	c.Assert(cfgOut.PruneTxnSleepTime(), tc.Equals, 102*time.Millisecond)
	c.Assert(cfgOut.QueryTracingThreshold(), tc.Equals, 103*time.Millisecond)
	c.Assert(cfgOut.MaxDebugLogDuration(), tc.Equals, 104*time.Millisecond)
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
