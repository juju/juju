// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"bytes"
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

// ---- get-controller-config tests ----

func (s *ControllerConfigSuite) TestGetMissingRuntimeConfigPath(c *tc.C) {
	cmd := new(getControllerConfigCommand)
	cmd.runtimeConfigPath = ""
	cmd.snapCommon = "/snap/common"
	err := cmd.Init(nil)
	c.Check(err, tc.ErrorMatches, "--runtime-config-path is required")
}

func (s *ControllerConfigSuite) TestGetMissingSnapCommon(c *tc.C) {
	cmd := new(getControllerConfigCommand)
	cmd.runtimeConfigPath = "/some/path"
	cmd.snapCommon = ""
	err := cmd.Init(nil)
	c.Check(err, tc.ErrorMatches, "--snap-common is required")
}

func (s *ControllerConfigSuite) TestGetExtraArgsRejected(c *tc.C) {
	cmd := new(getControllerConfigCommand)
	cmd.runtimeConfigPath = "/some/path"
	cmd.snapCommon = "/snap/common"
	err := cmd.Init([]string{"extra"})
	c.Check(err, tc.ErrorMatches, "unrecognized args.*")
}

func (s *ControllerConfigSuite) TestGetInfo(c *tc.C) {
	cmd := new(getControllerConfigCommand)
	info := cmd.Info()
	c.Check(info.Name, tc.Equals, "get-controller-config")
	c.Check(info.Purpose, tc.Not(tc.Equals), "")
}

func (s *ControllerConfigSuite) TestGetLoggingOverride_FromRuntimeConf(c *tc.C) {
	dir := c.MkDir()
	snapCommon := c.MkDir()
	runtimePath := filepath.Join(dir, controllerruntimeconfig.Filename)

	cfg := validConfig()
	cfg.LoggingOverride = "juju.worker=DEBUG"
	err := controllerruntimeconfig.WriteControllerRuntimeConfig(runtimePath, cfg)
	c.Assert(err, tc.ErrorIsNil)

	cmd := new(getControllerConfigCommand)
	cmd.runtimeConfigPath = runtimePath
	cmd.snapCommon = snapCommon
	err = cmd.Init(nil)
	c.Assert(err, tc.ErrorIsNil)

	ctx := newTestContext()
	err = cmd.Run(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ctx.Stdout.(*bytes.Buffer).String(), tc.Equals, "juju.worker=DEBUG\n")
}

func (s *ControllerConfigSuite) TestGetLoggingOverride_FromDeferredState(c *tc.C) {
	dir := c.MkDir()
	snapCommon := c.MkDir()
	runtimePath := filepath.Join(dir, controllerruntimeconfig.Filename)

	err := controllerruntimeconfig.WriteDeferredLoggingOverride(snapCommon, "juju.bootstrap=TRACE")
	c.Assert(err, tc.ErrorIsNil)

	cmd := new(getControllerConfigCommand)
	cmd.runtimeConfigPath = runtimePath
	cmd.snapCommon = snapCommon
	err = cmd.Init(nil)
	c.Assert(err, tc.ErrorIsNil)

	ctx := newTestContext()
	err = cmd.Run(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ctx.Stdout.(*bytes.Buffer).String(), tc.Equals, "juju.bootstrap=TRACE\n")
}

func (s *ControllerConfigSuite) TestGetLoggingOverride_EmptyWhenBothMissing(c *tc.C) {
	dir := c.MkDir()
	snapCommon := c.MkDir()
	runtimePath := filepath.Join(dir, controllerruntimeconfig.Filename)

	cmd := new(getControllerConfigCommand)
	cmd.runtimeConfigPath = runtimePath
	cmd.snapCommon = snapCommon
	err := cmd.Init(nil)
	c.Assert(err, tc.ErrorIsNil)

	ctx := newTestContext()
	err = cmd.Run(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ctx.Stdout.(*bytes.Buffer).String(), tc.Equals, "\n")
}

// ---- set-controller-config tests ----

func (s *ControllerConfigSuite) TestSetMissingRuntimeConfigPath(c *tc.C) {
	cmd := new(setControllerConfigCommand)
	cmd.runtimeConfigPath = ""
	cmd.snapCommon = "/snap/common"
	err := cmd.Init(nil)
	c.Check(err, tc.ErrorMatches, "--runtime-config-path is required")
}

func (s *ControllerConfigSuite) TestSetMissingSnapCommon(c *tc.C) {
	cmd := new(setControllerConfigCommand)
	cmd.runtimeConfigPath = "/some/path"
	cmd.snapCommon = ""
	err := cmd.Init(nil)
	c.Check(err, tc.ErrorMatches, "--snap-common is required")
}

func (s *ControllerConfigSuite) TestSetExtraArgsRejected(c *tc.C) {
	cmd := new(setControllerConfigCommand)
	cmd.runtimeConfigPath = "/some/path"
	cmd.snapCommon = "/snap/common"
	err := cmd.Init([]string{"extra"})
	c.Check(err, tc.ErrorMatches, "unrecognized args.*")
}

func (s *ControllerConfigSuite) TestSetInfo(c *tc.C) {
	cmd := new(setControllerConfigCommand)
	info := cmd.Info()
	c.Check(info.Name, tc.Equals, "set-controller-config")
	c.Check(info.Purpose, tc.Not(tc.Equals), "")
}

func (s *ControllerConfigSuite) TestSetApplyLoggingOverride_WhenRuntimeConfExists(c *tc.C) {
	dir := c.MkDir()
	snapCommon := c.MkDir()
	runtimePath := filepath.Join(dir, controllerruntimeconfig.Filename)

	cfg := validConfig()
	err := controllerruntimeconfig.WriteControllerRuntimeConfig(runtimePath, cfg)
	c.Assert(err, tc.ErrorIsNil)

	cmd := new(setControllerConfigCommand)
	cmd.loggingOverride = "juju.bootstrap=TRACE"
	cmd.runtimeConfigPath = runtimePath
	cmd.snapCommon = snapCommon
	err = cmd.Init(nil)
	c.Assert(err, tc.ErrorIsNil)

	ctx := newTestContext()
	err = cmd.Run(ctx)
	c.Assert(err, tc.ErrorIsNil)

	got, err := controllerruntimeconfig.ReadControllerRuntimeConfig(runtimePath)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.LoggingOverride, tc.Equals, "juju.bootstrap=TRACE")

	deferredVal, err := controllerruntimeconfig.ReadDeferredLoggingOverride(snapCommon)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(deferredVal, tc.Equals, "juju.bootstrap=TRACE")
}

func (s *ControllerConfigSuite) TestSetClearLoggingOverride_WhenRuntimeConfExists(c *tc.C) {
	dir := c.MkDir()
	snapCommon := c.MkDir()
	runtimePath := filepath.Join(dir, controllerruntimeconfig.Filename)

	cfg := validConfig()
	cfg.LoggingOverride = "juju.worker=DEBUG"
	err := controllerruntimeconfig.WriteControllerRuntimeConfig(runtimePath, cfg)
	c.Assert(err, tc.ErrorIsNil)

	cmd := new(setControllerConfigCommand)
	cmd.loggingOverride = ""
	cmd.runtimeConfigPath = runtimePath
	cmd.snapCommon = snapCommon
	err = cmd.Init(nil)
	c.Assert(err, tc.ErrorIsNil)

	ctx := newTestContext()
	err = cmd.Run(ctx)
	c.Assert(err, tc.ErrorIsNil)

	got, err := controllerruntimeconfig.ReadControllerRuntimeConfig(runtimePath)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.LoggingOverride, tc.Equals, "")

	deferredVal, err := controllerruntimeconfig.ReadDeferredLoggingOverride(snapCommon)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(deferredVal, tc.Equals, "")
}

func (s *ControllerConfigSuite) TestSetDeferLoggingOverride_WhenRuntimeConfMissing(c *tc.C) {
	dir := c.MkDir()
	snapCommon := c.MkDir()
	runtimePath := filepath.Join(dir, controllerruntimeconfig.Filename)

	cmd := new(setControllerConfigCommand)
	cmd.loggingOverride = "juju.bootstrap=TRACE"
	cmd.runtimeConfigPath = runtimePath
	cmd.snapCommon = snapCommon
	err := cmd.Init(nil)
	c.Assert(err, tc.ErrorIsNil)

	ctx := newTestContext()
	err = cmd.Run(ctx)
	c.Assert(err, tc.ErrorIsNil)

	val, err := controllerruntimeconfig.ReadDeferredLoggingOverride(snapCommon)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(val, tc.Equals, "juju.bootstrap=TRACE")

	_, err = os.Stat(runtimePath)
	c.Check(os.IsNotExist(err), tc.IsTrue)
}

func (s *ControllerConfigSuite) TestSetClearDeferredOverride_WhenRuntimeConfMissing(c *tc.C) {
	dir := c.MkDir()
	snapCommon := c.MkDir()
	runtimePath := filepath.Join(dir, controllerruntimeconfig.Filename)

	err := controllerruntimeconfig.WriteDeferredLoggingOverride(snapCommon, "old-value")
	c.Assert(err, tc.ErrorIsNil)

	cmd := new(setControllerConfigCommand)
	cmd.loggingOverride = ""
	cmd.runtimeConfigPath = runtimePath
	cmd.snapCommon = snapCommon
	err = cmd.Init(nil)
	c.Assert(err, tc.ErrorIsNil)

	ctx := newTestContext()
	err = cmd.Run(ctx)
	c.Assert(err, tc.ErrorIsNil)

	val, err := controllerruntimeconfig.ReadDeferredLoggingOverride(snapCommon)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(val, tc.Equals, "")
}

func (s *ControllerConfigSuite) TestSetPassesValidationForSupportedKey(c *tc.C) {
	dir := c.MkDir()
	snapCommon := c.MkDir()
	runtimePath := filepath.Join(dir, controllerruntimeconfig.Filename)

	cfg := validConfig()
	err := controllerruntimeconfig.WriteControllerRuntimeConfig(runtimePath, cfg)
	c.Assert(err, tc.ErrorIsNil)

	cmd := new(setControllerConfigCommand)
	cmd.loggingOverride = "juju.worker=TRACE"
	cmd.runtimeConfigPath = runtimePath
	cmd.snapCommon = snapCommon
	err = cmd.Init(nil)
	c.Assert(err, tc.ErrorIsNil)

	ctx := newTestContext()
	err = cmd.Run(ctx)
	c.Assert(err, tc.ErrorIsNil)

	got, err := controllerruntimeconfig.ReadControllerRuntimeConfig(runtimePath)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.LoggingOverride, tc.Equals, "juju.worker=TRACE")
}

func (s *ControllerConfigSuite) TestSetStatErrorNotIsNotExist(c *tc.C) {
	dir := c.MkDir()
	snapCommon := c.MkDir()

	blocker := filepath.Join(dir, "not-a-directory")
	err := os.WriteFile(blocker, nil, 0o644)
	c.Assert(err, tc.ErrorIsNil)

	runtimePath := filepath.Join(blocker, controllerruntimeconfig.Filename)

	cmd := new(setControllerConfigCommand)
	cmd.loggingOverride = "juju.bootstrap=TRACE"
	cmd.runtimeConfigPath = runtimePath
	cmd.snapCommon = snapCommon
	err = cmd.Init(nil)
	c.Assert(err, tc.ErrorIsNil)

	ctx := newTestContext()
	err = cmd.Run(ctx)
	c.Assert(err, tc.ErrorMatches, `checking runtime config.*`)
}

func validConfig() controllerruntimeconfig.ControllerRuntimeConfig {
	return controllerruntimeconfig.ControllerRuntimeConfig{
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
	}
}
