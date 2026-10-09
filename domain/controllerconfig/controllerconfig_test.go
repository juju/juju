// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllerconfig

import (
	"testing"
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

func TestControllerconfigSuite(t *testing.T) {
	tc.Run(t, &controllerconfigSuite{})
}

func (s *controllerconfigSuite) TestControllerConfigRoundTrips(c *tc.C) {
	st := domainstate.NewState(s.TxnRunnerFactory())
	srv := service.NewService(st)

	cfgMap := map[string]any{
		controller.AuditingEnabled:        true,
		controller.AuditLogCaptureArgs:    false,
		controller.AuditLogMaxBackups:     10,
		controller.PublicDNSAddress:       "controller.test.com:1234",
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

	c.Check(cfgOut.AuditingEnabled(), tc.IsTrue)
	c.Check(cfgOut.AuditLogCaptureArgs(), tc.IsFalse)
	c.Check(cfgOut.AuditLogMaxBackups(), tc.Equals, 10)
	c.Check(cfgOut.PublicDNSAddress(), tc.Equals, "controller.test.com:1234")
	c.Check(cfgOut.MigrationMinionWaitMax(), tc.Equals, 101*time.Millisecond)
	c.Check(cfgOut.PruneTxnSleepTime(), tc.Equals, 102*time.Millisecond)
	c.Check(cfgOut.QueryTracingThreshold(), tc.Equals, 103*time.Millisecond)
	c.Check(cfgOut.MaxDebugLogDuration(), tc.Equals, 104*time.Millisecond)

	c.Check(cfgOut.APIPort(), tc.Equals, 17070)
}

func (s *controllerconfigSuite) TestControllerConfigAPIPortRoundTrip(c *tc.C) {
	st := domainstate.NewState(s.TxnRunnerFactory())
	srv := service.NewService(st)

	cfgMap := map[string]any{
		controller.APIPort:                17071,
		controller.AuditingEnabled:        true,
		controller.AuditLogCaptureArgs:    false,
		controller.AuditLogMaxBackups:     10,
		controller.PublicDNSAddress:       "controller.test.com:1234",
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

	c.Check(cfgOut.APIPort(), tc.Equals, 17071)
}

func (s *controllerconfigSuite) TestUniqueValueGettersMatchControllerConfig(c *tc.C) {
	st := domainstate.NewState(s.TxnRunnerFactory())
	srv := service.NewService(st)

	cfgIn, err := controller.NewConfig(
		jujutesting.ControllerTag.Id(),
		jujutesting.CACert,
		map[string]any{
			controller.APIPort:             17071,
			controller.SSHServerPort:       17023,
			controller.JujuManagementSpace: "management",
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	controllerModelUUID := coremodel.UUID(jujutesting.ModelTag.Id())
	err = bootstrap.InsertInitialControllerConfig(cfgIn, controllerModelUUID)(c.Context(), s.TxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	cfg, err := srv.ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	sshPort, err := srv.GetSSHServerPort(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sshPort, tc.Equals, cfg.SSHServerPort())

	managementSpace, apiPort, err := srv.GetManagementSpaceAndAPIPort(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(managementSpace, tc.Equals, cfg.JujuManagementSpace())
	c.Check(apiPort, tc.Equals, cfg.APIPort())
}

func (s *controllerconfigSuite) TestUniqueValueGettersMatchControllerConfigDefaults(c *tc.C) {
	st := domainstate.NewState(s.TxnRunnerFactory())
	srv := service.NewService(st)

	cfgIn, err := controller.NewConfig(
		jujutesting.ControllerTag.Id(),
		jujutesting.CACert,
		nil,
	)
	c.Assert(err, tc.ErrorIsNil)

	controllerModelUUID := coremodel.UUID(jujutesting.ModelTag.Id())
	err = bootstrap.InsertInitialControllerConfig(cfgIn, controllerModelUUID)(c.Context(), s.TxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	cfg, err := srv.ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	sshPort, err := srv.GetSSHServerPort(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sshPort, tc.Equals, cfg.SSHServerPort())

	managementSpace, apiPort, err := srv.GetManagementSpaceAndAPIPort(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(managementSpace, tc.Equals, cfg.JujuManagementSpace())
	c.Check(apiPort, tc.Equals, cfg.APIPort())
}

func (s *controllerconfigSuite) TestUniqueValueGettersMatchControllerConfigErrors(c *tc.C) {
	st := domainstate.NewState(s.TxnRunnerFactory())
	srv := service.NewService(st)

	cfgIn, err := controller.NewConfig(
		jujutesting.ControllerTag.Id(),
		jujutesting.CACert,
		nil,
	)
	c.Assert(err, tc.ErrorIsNil)

	controllerModelUUID := coremodel.UUID(jujutesting.ModelTag.Id())
	err = bootstrap.InsertInitialControllerConfig(cfgIn, controllerModelUUID)(c.Context(), s.TxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	tests := []struct {
		name   string
		values map[string]string
		get    func(*service.Service) error
	}{
		{
			name: "invalid API port",
			values: map[string]string{
				controller.APIPort: "not-a-port",
			},
			get: func(srv *service.Service) error {
				_, _, err := srv.GetManagementSpaceAndAPIPort(c.Context())
				return err
			},
		},
		{
			name: "invalid management space",
			values: map[string]string{
				controller.JujuManagementSpace: "invalid space",
			},
			get: func(srv *service.Service) error {
				_, _, err := srv.GetManagementSpaceAndAPIPort(c.Context())
				return err
			},
		},
		{
			name: "invalid SSH port",
			values: map[string]string{
				controller.SSHServerPort: "not-a-port",
			},
			get: func(srv *service.Service) error {
				_, err := srv.GetSSHServerPort(c.Context())
				return err
			},
		},
		{
			name: "negative SSH port",
			values: map[string]string{
				controller.SSHServerPort: "-1",
			},
			get: func(srv *service.Service) error {
				_, err := srv.GetSSHServerPort(c.Context())
				return err
			},
		},
		{
			name: "SSH port matches API port",
			values: map[string]string{
				controller.SSHServerPort: "17070",
			},
			get: func(srv *service.Service) error {
				_, err := srv.GetSSHServerPort(c.Context())
				return err
			},
		},
	}

	for _, test := range tests {
		c.Logf("checking %s", test.name)
		err = st.UpdateControllerConfig(c.Context(), test.values, nil)
		c.Assert(err, tc.ErrorIsNil)

		_, fullConfigErr := srv.ControllerConfig(c.Context())
		c.Check(fullConfigErr, tc.ErrorMatches, `.+`)
		c.Check(test.get(srv), tc.ErrorMatches, `.+`)

		err = st.UpdateControllerConfig(c.Context(), map[string]string{
			controller.APIPort:             "17070",
			controller.SSHServerPort:       "17022",
			controller.JujuManagementSpace: "",
		}, nil)
		c.Assert(err, tc.ErrorIsNil)
	}
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
