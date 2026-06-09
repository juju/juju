// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllerruntimeconfig_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/juju/tc"
	"go.uber.org/goleak"

	"github.com/juju/juju/internal/controllerruntimeconfig"
	"github.com/juju/juju/internal/testhelpers"
)

type configSuite struct {
	testhelpers.IsolationSuite
}

func TestConfigSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &configSuite{})
}

func validConfig() controllerruntimeconfig.ControllerRuntimeConfig {
	return controllerruntimeconfig.ControllerRuntimeConfig{
		ControllerID:          "0",
		ControllerUUID:        "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		ControllerModelUUID:   "feedface-dead-beef-cafe-c0ffee000000",
		DataDir:               "/var/lib/juju",
		LogDir:                "/var/log/juju",
		QueryTracingEnabled:   true,
		QueryTracingThreshold: time.Second,
		DqliteBusyTimeout:     2 * time.Second,
		CACert:                "ca-cert-pem",
		CAPrivateKey:          "ca-private-key-pem",
		ControllerCert:        "controller-cert-pem",
		ControllerPrivateKey:  "controller-private-key-pem",
	}
}

// TestValidate_ValidConfig ensures a correctly populated config passes
// validation.
func (s *configSuite) TestValidate_ValidConfig(c *tc.C) {
	cfg := validConfig()
	c.Assert(cfg.Validate(), tc.ErrorIsNil)
}

// TestValidate_ZeroDqlitePortIsValid ensures DqlitePort == 0 (use default)
// is accepted.
func (s *configSuite) TestValidate_ZeroDqlitePortIsValid(c *tc.C) {
	cfg := validConfig()
	cfg.DqlitePort = 0
	c.Assert(cfg.Validate(), tc.ErrorIsNil)
}

// TestValidate_NonZeroDqlitePortIsValid ensures a valid non-zero Dqlite port
// is accepted.
func (s *configSuite) TestValidate_NonZeroDqlitePortIsValid(c *tc.C) {
	cfg := validConfig()
	cfg.DqlitePort = 17666
	c.Assert(cfg.Validate(), tc.ErrorIsNil)
}

// TestValidate_InvalidControllerID ensures a non-numeric controller ID is
// rejected.
func (s *configSuite) TestValidate_InvalidControllerID(c *tc.C) {
	cfg := validConfig()
	cfg.ControllerID = "not-numeric"
	c.Check(cfg.Validate(), tc.ErrorMatches, `controller ID "not-numeric" not valid`)
}

// TestValidate_EmptyControllerID ensures an empty controller ID is rejected.
func (s *configSuite) TestValidate_EmptyControllerID(c *tc.C) {
	cfg := validConfig()
	cfg.ControllerID = ""
	c.Check(cfg.Validate(), tc.ErrorMatches, `controller ID "" not valid`)
}

// TestValidate_EmptyControllerUUID ensures an empty controller UUID is
// rejected.
func (s *configSuite) TestValidate_EmptyControllerUUID(c *tc.C) {
	cfg := validConfig()
	cfg.ControllerUUID = ""
	c.Check(cfg.Validate(), tc.ErrorMatches, `controller UUID "" not valid`)
}

// TestValidate_EmptyControllerModelUUID ensures an empty controller model UUID
// is rejected.
func (s *configSuite) TestValidate_EmptyControllerModelUUID(c *tc.C) {
	cfg := validConfig()
	cfg.ControllerModelUUID = ""
	c.Check(cfg.Validate(), tc.ErrorMatches, `controller model UUID "" not valid`)
}

// TestValidate_EmptyDataDir ensures an empty data dir is rejected.
func (s *configSuite) TestValidate_EmptyDataDir(c *tc.C) {
	cfg := validConfig()
	cfg.DataDir = ""
	c.Check(cfg.Validate(), tc.ErrorMatches, `empty data-dir not valid`)
}

// TestValidate_EmptyLogDir ensures an empty log dir is rejected.
func (s *configSuite) TestValidate_EmptyLogDir(c *tc.C) {
	cfg := validConfig()
	cfg.LogDir = ""
	c.Check(cfg.Validate(), tc.ErrorMatches, `empty log-dir not valid`)
}

// TestValidate_InvalidDqlitePort ensures an out-of-range Dqlite port is
// rejected.
func (s *configSuite) TestValidate_InvalidDqlitePort(c *tc.C) {
	cfg := validConfig()
	cfg.DqlitePort = 99999
	c.Check(cfg.Validate(), tc.ErrorMatches, `dqlite port 99999 not valid`)
}

// TestValidate_NegativeQueryTracingThreshold ensures a negative threshold is
// rejected.
func (s *configSuite) TestValidate_NegativeQueryTracingThreshold(c *tc.C) {
	cfg := validConfig()
	cfg.QueryTracingThreshold = -time.Second
	c.Check(cfg.Validate(), tc.ErrorMatches, `negative query-tracing-threshold not valid`)
}

// TestValidate_ZeroQueryTracingThresholdIsValid ensures 0 threshold (log all)
// is accepted.
func (s *configSuite) TestValidate_ZeroQueryTracingThresholdIsValid(c *tc.C) {
	cfg := validConfig()
	cfg.QueryTracingThreshold = 0
	c.Assert(cfg.Validate(), tc.ErrorIsNil)
}

// TestValidate_NegativeDqliteBusyTimeout ensures a negative busy timeout is
// rejected.
func (s *configSuite) TestValidate_NegativeDqliteBusyTimeout(c *tc.C) {
	cfg := validConfig()
	cfg.DqliteBusyTimeout = -time.Second
	c.Check(cfg.Validate(), tc.ErrorMatches, `negative dqlite-busy-timeout not valid`)
}

// TestValidate_EmptyCACert ensures an empty CA cert is rejected.
func (s *configSuite) TestValidate_EmptyCACert(c *tc.C) {
	cfg := validConfig()
	cfg.CACert = ""
	c.Check(cfg.Validate(), tc.ErrorMatches, `empty ca-cert not valid`)
}

// TestValidate_EmptyCAPrivateKey ensures an empty CA private key is rejected.
func (s *configSuite) TestValidate_EmptyCAPrivateKey(c *tc.C) {
	cfg := validConfig()
	cfg.CAPrivateKey = ""
	c.Check(cfg.Validate(), tc.ErrorMatches, `empty ca-private-key not valid`)
}

// TestValidate_EmptyControllerCert ensures an empty controller cert is
// rejected.
func (s *configSuite) TestValidate_EmptyControllerCert(c *tc.C) {
	cfg := validConfig()
	cfg.ControllerCert = ""
	c.Check(cfg.Validate(), tc.ErrorMatches, `empty controller-cert not valid`)
}

// TestValidate_EmptyControllerPrivateKey ensures an empty private key is
// rejected.
func (s *configSuite) TestValidate_EmptyControllerPrivateKey(c *tc.C) {
	cfg := validConfig()
	cfg.ControllerPrivateKey = ""
	c.Check(cfg.Validate(), tc.ErrorMatches, `empty controller-private-key not valid`)
}

// TestWriteAndReadRoundTrip ensures the write/read cycle preserves all fields.
func (s *configSuite) TestWriteAndReadRoundTrip(c *tc.C) {
	dir := c.MkDir()
	path := filepath.Join(dir, controllerruntimeconfig.Filename)
	cfg := validConfig()
	cfg.DqlitePort = 17666

	err := controllerruntimeconfig.WriteControllerRuntimeConfig(path, cfg)
	c.Assert(err, tc.ErrorIsNil)

	got, err := controllerruntimeconfig.ReadControllerRuntimeConfig(path)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, cfg)
}

// TestWriteAndReadRoundTrip_SystemIdentity ensures SystemIdentity is preserved
// in the write/read round trip.
func (s *configSuite) TestWriteAndReadRoundTrip_SystemIdentity(c *tc.C) {
	dir := c.MkDir()
	path := filepath.Join(dir, controllerruntimeconfig.Filename)
	cfg := validConfig()
	cfg.SystemIdentity = "-----BEGIN OPENSSH PRIVATE KEY-----\ntest-ssh-key\n-----END OPENSSH PRIVATE KEY-----\n"

	err := controllerruntimeconfig.WriteControllerRuntimeConfig(path, cfg)
	c.Assert(err, tc.ErrorIsNil)

	got, err := controllerruntimeconfig.ReadControllerRuntimeConfig(path)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.SystemIdentity, tc.Equals, cfg.SystemIdentity)
}

// TestWriteAndReadRoundTrip_EmptySystemIdentityOmitted ensures that an empty
// SystemIdentity is not written to the YAML output, allowing existing runtime
// config files without the field to read back cleanly.
func (s *configSuite) TestWriteAndReadRoundTrip_EmptySystemIdentityOmitted(c *tc.C) {
	dir := c.MkDir()
	path := filepath.Join(dir, controllerruntimeconfig.Filename)
	cfg := validConfig()
	// SystemIdentity left empty.

	err := controllerruntimeconfig.WriteControllerRuntimeConfig(path, cfg)
	c.Assert(err, tc.ErrorIsNil)

	got, err := controllerruntimeconfig.ReadControllerRuntimeConfig(path)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.SystemIdentity, tc.Equals, "")
}

// TestWriteAndReadRoundTrip_AllNodeManagerFields confirms that all fields
// required for NodeManager are preserved after a round-trip.
func (s *configSuite) TestWriteAndReadRoundTrip_AllNodeManagerFields(c *tc.C) {
	dir := c.MkDir()
	path := filepath.Join(dir, controllerruntimeconfig.Filename)
	cfg := controllerruntimeconfig.ControllerRuntimeConfig{
		ControllerID:          "0",
		ControllerUUID:        "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		ControllerModelUUID:   "feedface-dead-beef-cafe-c0ffee000000",
		DataDir:               "/var/lib/juju",
		LogDir:                "/var/log/juju",
		DqlitePort:            0, // default
		QueryTracingEnabled:   true,
		QueryTracingThreshold: 500 * time.Millisecond,
		DqliteBusyTimeout:     3 * time.Second,
		CACert:                "ca-cert-pem-data",
		CAPrivateKey:          "ca-private-key-pem-data",
		ControllerCert:        "controller-cert-pem-data",
		ControllerPrivateKey:  "controller-private-key-pem-data",
	}

	err := controllerruntimeconfig.WriteControllerRuntimeConfig(path, cfg)
	c.Assert(err, tc.ErrorIsNil)

	got, err := controllerruntimeconfig.ReadControllerRuntimeConfig(path)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.ControllerID, tc.Equals, cfg.ControllerID)
	c.Check(got.ControllerUUID, tc.Equals, cfg.ControllerUUID)
	c.Check(got.ControllerModelUUID, tc.Equals, cfg.ControllerModelUUID)
	c.Check(got.DataDir, tc.Equals, cfg.DataDir)
	c.Check(got.LogDir, tc.Equals, cfg.LogDir)
	c.Check(got.CACert, tc.Equals, cfg.CACert)
	c.Check(got.CAPrivateKey, tc.Equals, cfg.CAPrivateKey)
	c.Check(got.ControllerCert, tc.Equals, cfg.ControllerCert)
	c.Check(got.ControllerPrivateKey, tc.Equals, cfg.ControllerPrivateKey)
	c.Check(got.DqliteBusyTimeout, tc.Equals, cfg.DqliteBusyTimeout)
	c.Check(got.DqlitePort, tc.Equals, cfg.DqlitePort)
	c.Check(got.QueryTracingEnabled, tc.Equals, cfg.QueryTracingEnabled)
	c.Check(got.QueryTracingThreshold, tc.Equals, cfg.QueryTracingThreshold)
}

// TestWrite_CreatesParentDirectory ensures WriteControllerRuntimeConfig
// creates the parent directory if it does not exist.
func (s *configSuite) TestWrite_CreatesParentDirectory(c *tc.C) {
	dir := c.MkDir()
	path := filepath.Join(dir, "agents", "controller-0",
		controllerruntimeconfig.Filename)
	cfg := validConfig()

	err := controllerruntimeconfig.WriteControllerRuntimeConfig(path, cfg)
	c.Assert(err, tc.ErrorIsNil)

	_, err = os.Stat(path)
	c.Assert(err, tc.ErrorIsNil)
}

// TestWrite_Permissions ensures the written file has 0600 permissions.
func (s *configSuite) TestWrite_Permissions(c *tc.C) {
	dir := c.MkDir()
	path := filepath.Join(dir,
		controllerruntimeconfig.Filename)
	cfg := validConfig()

	err := controllerruntimeconfig.WriteControllerRuntimeConfig(path, cfg)
	c.Assert(err, tc.ErrorIsNil)

	info, err := os.Stat(path)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.Mode().Perm(), tc.Equals, os.FileMode(0600))
}

// TestRead_MissingFile ensures ReadControllerRuntimeConfig returns an
// annotated error when the file does not exist.
func (s *configSuite) TestRead_MissingFile(c *tc.C) {
	_, err := controllerruntimeconfig.ReadControllerRuntimeConfig(
		"/nonexistent/path/runtime.conf")
	c.Assert(err, tc.ErrorMatches,
		`reading controller runtime config "/nonexistent/path/runtime.conf":.*`)
}

// TestRead_MalformedFile ensures ReadControllerRuntimeConfig returns an
// annotated parse error for malformed YAML.
func (s *configSuite) TestRead_MalformedFile(c *tc.C) {
	dir := c.MkDir()
	path := filepath.Join(dir, controllerruntimeconfig.Filename)
	err := os.WriteFile(path, []byte(":\tinvalid: yaml: content"), 0600)
	c.Assert(err, tc.ErrorIsNil)

	_, err = controllerruntimeconfig.ReadControllerRuntimeConfig(path)
	c.Assert(err, tc.ErrorMatches, `parsing controller runtime config.*`)
}

// TestRead_MissingRequiredField ensures ReadControllerRuntimeConfig returns a
// validation error naming the missing field.
func (s *configSuite) TestRead_MissingRequiredField(c *tc.C) {
	dir := c.MkDir()
	path := filepath.Join(dir, controllerruntimeconfig.Filename)
	// Write config without the required ca-cert field.
	content := `
controller-id: "0"
controller-uuid: "deadbeef-0bad-400d-8000-4b1d0d06f00d"
controller-model-uuid: "feedface-dead-beef-cafe-c0ffee000000"
data-dir: /var/lib/juju
log-dir: /var/log/juju
controller-cert: cert-pem
controller-private-key: key-pem
`[1:]
	err := os.WriteFile(path, []byte(content), 0600)
	c.Assert(err, tc.ErrorIsNil)

	_, err = controllerruntimeconfig.ReadControllerRuntimeConfig(path)
	c.Assert(err, tc.ErrorMatches, `validating controller runtime config.*ca-cert.*`)
}

// TestWriteAndReadRoundTrip_LogSinkRateLimits ensures the log-sink rate-limit
// fields are preserved in the write/read round trip when set.
func (s *configSuite) TestWriteAndReadRoundTrip_LogSinkRateLimits(c *tc.C) {
	dir := c.MkDir()
	path := filepath.Join(dir, controllerruntimeconfig.Filename)
	cfg := validConfig()
	cfg.LogSinkRateLimitBurst = 2000
	cfg.LogSinkRateLimitRefill = 5 * time.Millisecond

	err := controllerruntimeconfig.WriteControllerRuntimeConfig(path, cfg)
	c.Assert(err, tc.ErrorIsNil)

	got, err := controllerruntimeconfig.ReadControllerRuntimeConfig(path)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.LogSinkRateLimitBurst, tc.Equals, int64(2000))
	c.Check(got.LogSinkRateLimitRefill, tc.Equals, 5*time.Millisecond)
}

// TestWriteAndReadRoundTrip_LogSinkRateLimitsOmittedWhenZero ensures that
// zero-value log-sink rate-limit fields are omitted from YAML output and
// read back as zero, signalling "use defaults".
func (s *configSuite) TestWriteAndReadRoundTrip_LogSinkRateLimitsOmittedWhenZero(c *tc.C) {
	dir := c.MkDir()
	path := filepath.Join(dir, controllerruntimeconfig.Filename)
	cfg := validConfig()
	// LogSinkRateLimitBurst and LogSinkRateLimitRefill left at zero.

	err := controllerruntimeconfig.WriteControllerRuntimeConfig(path, cfg)
	c.Assert(err, tc.ErrorIsNil)

	got, err := controllerruntimeconfig.ReadControllerRuntimeConfig(path)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.LogSinkRateLimitBurst, tc.Equals, int64(0))
	c.Check(got.LogSinkRateLimitRefill, tc.Equals, time.Duration(0))
}

// TestConfigPath ensures the path helper returns the expected path.
func (s *configSuite) TestConfigPath(c *tc.C) {
	got := controllerruntimeconfig.ConfigPath(
		"/var/lib/juju/agents/controller-0")
	c.Check(got, tc.Equals,
		"/var/lib/juju/agents/controller-0/runtime.conf")
}

// TestRenderControllerRuntimeConfig ensures RenderControllerRuntimeConfig
// returns valid YAML that round-trips correctly.
func (s *configSuite) TestRenderControllerRuntimeConfig(c *tc.C) {
	cfg := validConfig()

	data, err := controllerruntimeconfig.RenderControllerRuntimeConfig(cfg)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(data, tc.Not(tc.HasLen), 0)

	// Write to a temp file and read back to confirm round-trip.
	dir := c.MkDir()
	path := filepath.Join(dir, controllerruntimeconfig.Filename)
	err = os.WriteFile(path, data, 0600)
	c.Assert(err, tc.ErrorIsNil)

	got, err := controllerruntimeconfig.ReadControllerRuntimeConfig(path)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, cfg)
}
