// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllerruntimeconfig_test

import (
	"os"
	"path/filepath"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/controllerruntimeconfig"
)

func (s *configSuite) TestValidateSnapConfigOverlay_Valid(c *tc.C) {
	vals := map[string]string{
		"logging-override": "juju.bootstrap=TRACE",
	}
	err := controllerruntimeconfig.ValidateSnapConfigOverlay(vals)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *configSuite) TestValidateSnapConfigOverlay_Empty(c *tc.C) {
	err := controllerruntimeconfig.ValidateSnapConfigOverlay(map[string]string{})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *configSuite) TestValidateSnapConfigOverlay_SingleUnsupported(c *tc.C) {
	vals := map[string]string{
		"query-tracing-threshold": "100ms",
	}
	err := controllerruntimeconfig.ValidateSnapConfigOverlay(vals)
	c.Assert(err, tc.ErrorMatches, `.*cannot apply snap config: 1 controller-database-owned key.*not supported through snap set in Phase 1.*`)
}

func (s *configSuite) TestValidateSnapConfigOverlay_MultipleUnsupported(c *tc.C) {
	vals := map[string]string{
		"query-tracing-threshold":   "100ms",
		"dqlite-busy-timeout":       "30s",
		"agent-logfile-max-size-mb": "64",
	}
	err := controllerruntimeconfig.ValidateSnapConfigOverlay(vals)
	c.Assert(err, tc.ErrorMatches, `.*cannot apply snap config: 3 controller-database-owned key.*not supported through snap set in Phase 1.*`)
}

func (s *configSuite) TestValidateSnapConfigOverlay_MixedSupportedAndUnsupported(c *tc.C) {
	vals := map[string]string{
		"logging-override":          "juju.bootstrap=TRACE",
		"agent-logfile-max-size-mb": "64",
		"agent-logfile-max-backups": "10",
	}
	err := controllerruntimeconfig.ValidateSnapConfigOverlay(vals)
	c.Assert(err, tc.ErrorMatches, `.*cannot apply snap config: 2 controller-database-owned key.*not supported through snap set in Phase 1.*`)
}

func (s *configSuite) TestApplySnapConfigOverlay_MutatesLoggingOverride(c *tc.C) {
	dir := c.MkDir()
	path := filepath.Join(dir, controllerruntimeconfig.Filename)
	cfg := validConfig()
	cfg.LoggingOverride = ""

	err := controllerruntimeconfig.WriteControllerRuntimeConfig(path, cfg)
	c.Assert(err, tc.ErrorIsNil)

	overlay := controllerruntimeconfig.SnapConfigOverlay{
		LoggingOverride: "juju.bootstrap=TRACE",
	}
	err = controllerruntimeconfig.ApplySnapConfigOverlay(path, overlay)
	c.Assert(err, tc.ErrorIsNil)

	got, err := controllerruntimeconfig.ReadControllerRuntimeConfig(path)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.LoggingOverride, tc.Equals, "juju.bootstrap=TRACE")
	// All other fields are preserved.
	c.Check(got.ControllerID, tc.Equals, cfg.ControllerID)
	c.Check(got.ControllerUUID, tc.Equals, cfg.ControllerUUID)
	c.Check(got.CACert, tc.Equals, cfg.CACert)
	c.Check(got.AgentPassword, tc.Equals, cfg.AgentPassword)
}

func (s *configSuite) TestApplySnapConfigOverlay_ClearsLoggingOverride(c *tc.C) {
	dir := c.MkDir()
	path := filepath.Join(dir, controllerruntimeconfig.Filename)
	cfg := validConfig()
	cfg.LoggingOverride = "juju.worker=DEBUG"

	err := controllerruntimeconfig.WriteControllerRuntimeConfig(path, cfg)
	c.Assert(err, tc.ErrorIsNil)

	overlay := controllerruntimeconfig.SnapConfigOverlay{
		LoggingOverride: "",
	}
	err = controllerruntimeconfig.ApplySnapConfigOverlay(path, overlay)
	c.Assert(err, tc.ErrorIsNil)

	got, err := controllerruntimeconfig.ReadControllerRuntimeConfig(path)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.LoggingOverride, tc.Equals, "")
}

func (s *configSuite) TestApplySnapConfigOverlay_MissingRuntimeConf(c *tc.C) {
	path := "/nonexistent/path/runtime.conf"
	overlay := controllerruntimeconfig.SnapConfigOverlay{
		LoggingOverride: "juju.bootstrap=TRACE",
	}
	err := controllerruntimeconfig.ApplySnapConfigOverlay(path, overlay)
	c.Assert(err, tc.ErrorMatches, `.*reading controller runtime config.*`)
}

func (s *configSuite) TestApplySnapConfigOverlay_PreservesOtherFields(c *tc.C) {
	dir := c.MkDir()
	path := filepath.Join(dir, controllerruntimeconfig.Filename)
	cfg := validConfig()
	cfg.DqliteBusyTimeout = 15 * time.Second
	cfg.QueryTracingThreshold = 50 * time.Millisecond

	err := controllerruntimeconfig.WriteControllerRuntimeConfig(path, cfg)
	c.Assert(err, tc.ErrorIsNil)

	overlay := controllerruntimeconfig.SnapConfigOverlay{
		LoggingOverride: "juju.bootstrap=TRACE",
	}
	err = controllerruntimeconfig.ApplySnapConfigOverlay(path, overlay)
	c.Assert(err, tc.ErrorIsNil)

	got, err := controllerruntimeconfig.ReadControllerRuntimeConfig(path)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.LoggingOverride, tc.Equals, "juju.bootstrap=TRACE")
	// Controller-database-owned fields are preserved unchanged.
	c.Check(got.DqliteBusyTimeout, tc.Equals, 15*time.Second)
	c.Check(got.QueryTracingThreshold, tc.Equals, 50*time.Millisecond)
}

func (s *configSuite) TestReadDeferredLoggingOverride_FileDoesNotExist(c *tc.C) {
	dir := c.MkDir()
	val, err := controllerruntimeconfig.ReadDeferredLoggingOverride(dir)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(val, tc.Equals, "")
}

func (s *configSuite) TestWriteAndReadDeferredLoggingOverride_RoundTrip(c *tc.C) {
	dir := c.MkDir()

	err := controllerruntimeconfig.WriteDeferredLoggingOverride(dir, "juju.bootstrap=TRACE")
	c.Assert(err, tc.ErrorIsNil)

	val, err := controllerruntimeconfig.ReadDeferredLoggingOverride(dir)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(val, tc.Equals, "juju.bootstrap=TRACE")
}

func (s *configSuite) TestWriteDeferredLoggingOverride_ClearsOnEmpty(c *tc.C) {
	dir := c.MkDir()

	err := controllerruntimeconfig.WriteDeferredLoggingOverride(dir, "juju.bootstrap=TRACE")
	c.Assert(err, tc.ErrorIsNil)

	err = controllerruntimeconfig.WriteDeferredLoggingOverride(dir, "")
	c.Assert(err, tc.ErrorIsNil)

	val, err := controllerruntimeconfig.ReadDeferredLoggingOverride(dir)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(val, tc.Equals, "")
}

func (s *configSuite) TestDeferredLoggingOverridePath(c *tc.C) {
	got := controllerruntimeconfig.DeferredLoggingOverridePath("/snap/common")
	c.Check(got, tc.Equals, "/snap/common/.snap-init/logging-override")
}

func (s *configSuite) TestWriteDeferredLoggingOverride_CreatesParentDir(c *tc.C) {
	dir := c.MkDir()
	snapCommon := filepath.Join(dir, "new", "sub")

	err := controllerruntimeconfig.WriteDeferredLoggingOverride(snapCommon, "juju.bootstrap=TRACE")
	c.Assert(err, tc.ErrorIsNil)

	val, err := controllerruntimeconfig.ReadDeferredLoggingOverride(snapCommon)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(val, tc.Equals, "juju.bootstrap=TRACE")

	// Verify parent directory exists with correct permissions.
	parentDir := filepath.Join(snapCommon, controllerruntimeconfig.SnapInitDir)
	info, err := os.Stat(parentDir)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.IsDir(), tc.IsTrue)
}
