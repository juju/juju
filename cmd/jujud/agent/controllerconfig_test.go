// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/controllerruntimeconfig"
	"github.com/juju/juju/internal/testhelpers"
)

type ControllerConfigSuite struct {
	testhelpers.IsolationSuite
}

func TestControllerConfigSuite(t *testing.T) {
	tc.Run(t, &ControllerConfigSuite{})
}

func (s *ControllerConfigSuite) TestMissingRuntimeConfigPath(c *tc.C) {
	cmd := new(controllerConfigCommand)
	cmd.runtimeConfigPath = ""
	cmd.snapCommon = "/snap/common"
	err := cmd.Init(nil)
	c.Check(err, tc.ErrorMatches, "--runtime-config-path is required")
}

func (s *ControllerConfigSuite) TestMissingSnapCommon(c *tc.C) {
	cmd := new(controllerConfigCommand)
	cmd.runtimeConfigPath = "/some/path"
	cmd.snapCommon = ""
	err := cmd.Init(nil)
	c.Check(err, tc.ErrorMatches, "--snap-common is required")
}

func (s *ControllerConfigSuite) TestExtraArgsRejected(c *tc.C) {
	cmd := new(controllerConfigCommand)
	cmd.runtimeConfigPath = "/some/path"
	cmd.snapCommon = "/snap/common"
	err := cmd.Init([]string{"extra"})
	c.Check(err, tc.ErrorMatches, "unrecognized args.*")
}

func (s *ControllerConfigSuite) TestInfo(c *tc.C) {
	cmd := new(controllerConfigCommand)
	info := cmd.Info()
	c.Check(info.Name, tc.Equals, "controller-config")
	c.Check(info.Purpose, tc.Not(tc.Equals), "")
}

func (s *ControllerConfigSuite) TestApplyLoggingOverride_WhenRuntimeConfExists(c *tc.C) {
	dir := c.MkDir()
	snapCommon := c.MkDir()
	runtimePath := filepath.Join(dir, controllerruntimeconfig.Filename)

	cfg := controllerruntimeconfig.ControllerRuntimeConfig{
		ControllerID:         "0",
		ControllerUUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		ControllerModelUUID:  "feedface-dead-beef-cafe-c0ffee000000",
		DataDir:              "/var/lib/juju",
		LogDir:               "/var/log/juju",
		APIPort:              17070,
		AgentPassword:        "agent-password",
		CACert:               "ca-cert-pem",
		CAPrivateKey:         "ca-private-key-pem",
		ControllerCert:       "controller-cert-pem",
		ControllerPrivateKey: "controller-private-key-pem",
		LoggingOverride:      "",
	}
	err := controllerruntimeconfig.WriteControllerRuntimeConfig(runtimePath, cfg)
	c.Assert(err, tc.ErrorIsNil)

	app := new(controllerConfigCommand)
	app.loggingOverride = "juju.bootstrap=TRACE"
	app.runtimeConfigPath = runtimePath
	app.snapCommon = snapCommon
	err = app.Init(nil)
	c.Assert(err, tc.ErrorIsNil)

	ctx := newTestContext()
	err = app.Run(ctx)
	c.Assert(err, tc.ErrorIsNil)

	got, err := controllerruntimeconfig.ReadControllerRuntimeConfig(runtimePath)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.LoggingOverride, tc.Equals, "juju.bootstrap=TRACE")
}

func (s *ControllerConfigSuite) TestClearLoggingOverride_WhenRuntimeConfExists(c *tc.C) {
	dir := c.MkDir()
	snapCommon := c.MkDir()
	runtimePath := filepath.Join(dir, controllerruntimeconfig.Filename)

	cfg := controllerruntimeconfig.ControllerRuntimeConfig{
		ControllerID:         "0",
		ControllerUUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		ControllerModelUUID:  "feedface-dead-beef-cafe-c0ffee000000",
		DataDir:              "/var/lib/juju",
		LogDir:               "/var/log/juju",
		APIPort:              17070,
		AgentPassword:        "agent-password",
		CACert:               "ca-cert-pem",
		CAPrivateKey:         "ca-private-key-pem",
		ControllerCert:       "controller-cert-pem",
		ControllerPrivateKey: "controller-private-key-pem",
		LoggingOverride:      "juju.worker=DEBUG",
	}
	err := controllerruntimeconfig.WriteControllerRuntimeConfig(runtimePath, cfg)
	c.Assert(err, tc.ErrorIsNil)

	app := new(controllerConfigCommand)
	app.loggingOverride = ""
	app.runtimeConfigPath = runtimePath
	app.snapCommon = snapCommon
	err = app.Init(nil)
	c.Assert(err, tc.ErrorIsNil)

	ctx := newTestContext()
	err = app.Run(ctx)
	c.Assert(err, tc.ErrorIsNil)

	got, err := controllerruntimeconfig.ReadControllerRuntimeConfig(runtimePath)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.LoggingOverride, tc.Equals, "")
}

func (s *ControllerConfigSuite) TestDeferLoggingOverride_WhenRuntimeConfMissing(c *tc.C) {
	dir := c.MkDir()
	snapCommon := c.MkDir()
	runtimePath := filepath.Join(dir, controllerruntimeconfig.Filename)
	// Do not create runtime.conf.

	app := new(controllerConfigCommand)
	app.loggingOverride = "juju.bootstrap=TRACE"
	app.runtimeConfigPath = runtimePath
	app.snapCommon = snapCommon
	err := app.Init(nil)
	c.Assert(err, tc.ErrorIsNil)

	ctx := newTestContext()
	err = app.Run(ctx)
	c.Assert(err, tc.ErrorIsNil)

	// The value should be deferred.
	val, err := controllerruntimeconfig.ReadDeferredLoggingOverride(snapCommon)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(val, tc.Equals, "juju.bootstrap=TRACE")

	// runtime.conf should not have been created.
	_, err = os.Stat(runtimePath)
	c.Check(os.IsNotExist(err), tc.IsTrue)
}

func (s *ControllerConfigSuite) TestClearDeferredOverride_WhenRuntimeConfMissing(c *tc.C) {
	dir := c.MkDir()
	snapCommon := c.MkDir()
	runtimePath := filepath.Join(dir, controllerruntimeconfig.Filename)

	// Pre-populate deferred state.
	err := controllerruntimeconfig.WriteDeferredLoggingOverride(snapCommon, "old-value")
	c.Assert(err, tc.ErrorIsNil)

	app := new(controllerConfigCommand)
	app.loggingOverride = ""
	app.runtimeConfigPath = runtimePath
	app.snapCommon = snapCommon
	err = app.Init(nil)
	c.Assert(err, tc.ErrorIsNil)

	ctx := newTestContext()
	err = app.Run(ctx)
	c.Assert(err, tc.ErrorIsNil)

	// The deferred file should be cleared.
	val, err := controllerruntimeconfig.ReadDeferredLoggingOverride(snapCommon)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(val, tc.Equals, "")
}
