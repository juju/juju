// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/internal/controllerruntimeconfig"
	"github.com/juju/juju/internal/testhelpers"
)

func newTestContext() *cmd.Context {
	return &cmd.Context{
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	}
}

type InitCommandSuite struct {
	testhelpers.IsolationSuite
}

func TestInitCommandSuite(t *testing.T) {
	tc.Run(t, &InitCommandSuite{})
}

// validStagedRuntimeConf returns a tokenized staged runtime configuration that
// will resolve and validate successfully when processed by the init command.
// snapData and snapCommon are the expected final snap paths.
func validStagedRuntimeConf(c *tc.C) []byte {
	cfg := controllerruntimeconfig.ControllerRuntimeConfig{
		ControllerID:         "0",
		ControllerUUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		ControllerModelUUID:  "feedface-dead-beef-cafe-c0ffee000000",
		DataDir:              "/placeholder", // will be replaced with token
		LogDir:               "/placeholder", // will be replaced with token
		APIPort:              17070,
		AgentPassword:        "agent-password",
		CACert:               "ca-cert-pem",
		CAPrivateKey:         "ca-private-key-pem",
		ControllerCert:       "controller-cert-pem",
		ControllerPrivateKey: "controller-private-key-pem",
		APIAddresses:         []string{"10.0.0.1:17070"},
	}
	data, err := controllerruntimeconfig.RenderStagedControllerRuntimeConfig(cfg)
	c.Assert(err, tc.ErrorIsNil)
	return data
}

func (s *InitCommandSuite) TestInfo(c *tc.C) {
	ic := &initCommand{}
	info := ic.Info()
	c.Check(info.Name, tc.Equals, "init")
	c.Check(info.Purpose, tc.Not(tc.Equals), "")
}

func (s *InitCommandSuite) TestMissingStagedDir(c *tc.C) {
	ic := &initCommand{}
	err := ic.Init(nil)
	c.Check(err, tc.ErrorMatches, "--staged-dir is required")
}

func (s *InitCommandSuite) TestStagedDirNotExist(c *tc.C) {
	ic := &initCommand{stagedDir: "/nonexistent/path"}
	err := ic.Init(nil)
	c.Check(err, tc.ErrorMatches, ".*no such file or directory.*")
}

func (s *InitCommandSuite) TestRunWithoutSNAP_DATA(c *tc.C) {
	s.PatchValue(&osGetenv, func(key string) string { return "" })
	ic := &initCommand{stagedDir: c.MkDir()}
	ctx := newTestContext()
	err := ic.Run(ctx)
	c.Check(err, tc.ErrorMatches, "SNAP_DATA is not set")
}

func (s *InitCommandSuite) TestRunWithoutSNAP_COMMON(c *tc.C) {
	s.PatchValue(&osGetenv, func(key string) string {
		if key == "SNAP_DATA" {
			return "/snap/data"
		}
		return ""
	})
	ic := &initCommand{stagedDir: c.MkDir()}
	ctx := newTestContext()
	err := ic.Run(ctx)
	c.Check(err, tc.ErrorMatches, "SNAP_COMMON is not set")
}

func (s *InitCommandSuite) TestMissingRuntimeConf(c *tc.C) {
	stagedDir := c.MkDir()
	snapData := c.MkDir()
	snapCommon := c.MkDir()

	s.PatchValue(&osGetenv, func(key string) string {
		switch key {
		case "SNAP_DATA":
			return snapData
		case "SNAP_COMMON":
			return snapCommon
		}
		return ""
	})

	ic := &initCommand{stagedDir: stagedDir}
	ctx := newTestContext()
	err := ic.Run(ctx)
	c.Check(err, tc.ErrorMatches, `.*runtime.conf.*does not exist.*`)
}

func (s *InitCommandSuite) TestMissingBootstrapParams(c *tc.C) {
	stagedDir := c.MkDir()
	snapData := c.MkDir()
	snapCommon := c.MkDir()

	// Create a valid staged runtime.conf with tokens so it resolves
	// successfully; do not create bootstrap-params.
	stagedContent := validStagedRuntimeConf(c)
	err := os.WriteFile(filepath.Join(stagedDir, controllerruntimeconfig.Filename), stagedContent, 0o644)
	c.Assert(err, tc.ErrorIsNil)

	s.PatchValue(&osGetenv, func(key string) string {
		switch key {
		case "SNAP_DATA":
			return snapData
		case "SNAP_COMMON":
			return snapCommon
		}
		return ""
	})

	ic := &initCommand{stagedDir: stagedDir}
	ctx := newTestContext()
	err = ic.Run(ctx)
	c.Check(err, tc.ErrorMatches, `.*bootstrap-params.*does not exist.*`)
}

func (s *InitCommandSuite) TestSuccessfulInit(c *tc.C) {
	stagedDir := c.MkDir()
	snapData := c.MkDir()
	snapCommon := c.MkDir()

	stagedContent := validStagedRuntimeConf(c)
	bootstrapContent := `{"controller-config":{}}`

	err := os.WriteFile(filepath.Join(stagedDir, controllerruntimeconfig.Filename), stagedContent, 0o644)
	c.Assert(err, tc.ErrorIsNil)
	err = os.WriteFile(filepath.Join(stagedDir, controllerruntimeconfig.FileNameBootstrapParams), []byte(bootstrapContent), 0o644)
	c.Assert(err, tc.ErrorIsNil)

	s.PatchValue(&osGetenv, func(key string) string {
		switch key {
		case "SNAP_DATA":
			return snapData
		case "SNAP_COMMON":
			return snapCommon
		}
		return ""
	})

	ic := &initCommand{stagedDir: stagedDir}
	ctx := newTestContext()
	err = ic.Run(ctx)
	c.Assert(err, tc.ErrorIsNil)

	// Verify runtime.conf was written to $SNAP_DATA with resolved snap paths.
	runtimeDst := filepath.Join(snapData, controllerAgentDir, controllerruntimeconfig.Filename)
	data, err := os.ReadFile(runtimeDst)
	c.Assert(err, tc.ErrorIsNil)
	runtimeStr := string(data)

	// The token values must have been replaced by the actual snap paths.
	c.Check(runtimeStr, tc.Not(tc.Contains), controllerruntimeconfig.TokenSnapData)
	c.Check(runtimeStr, tc.Not(tc.Contains), controllerruntimeconfig.TokenSnapCommon)
	c.Check(runtimeStr, tc.Contains, "data-dir: "+snapData)
	c.Check(runtimeStr, tc.Contains, "log-dir: "+snapCommon+"/var/log/juju")

	// Verify runtime.conf file permissions.
	info, err := os.Stat(runtimeDst)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.Mode().Perm(), tc.Equals, os.FileMode(0o600))

	// Verify runtime.conf parent directory permissions.
	parentInfo, err := os.Stat(filepath.Dir(runtimeDst))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(parentInfo.Mode().Perm(), tc.Equals, os.FileMode(0o700))

	// Verify bootstrap-params was written to $SNAP_COMMON byte-for-byte.
	bootstrapDst := filepath.Join(snapCommon, controllerruntimeconfig.FileNameBootstrapParams)
	data, err = os.ReadFile(bootstrapDst)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(data), tc.Equals, bootstrapContent)

	// Verify bootstrap-params file permissions.
	info, err = os.Stat(bootstrapDst)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.Mode().Perm(), tc.Equals, os.FileMode(0o600))
}

// TestTokenResolutionFourPaths verifies that the four documented snap-path
// tokens are resolved in the final runtime.conf and that no credential or
// non-path field is modified.
func (s *InitCommandSuite) TestTokenResolutionFourPaths(c *tc.C) {
	stagedDir := c.MkDir()
	snapData := c.MkDir()
	snapCommon := c.MkDir()

	bootstrapContent := `{"controller-config":{}}`
	stagedContent := validStagedRuntimeConf(c)
	err := os.WriteFile(filepath.Join(stagedDir, controllerruntimeconfig.Filename), stagedContent, 0o644)
	c.Assert(err, tc.ErrorIsNil)
	err = os.WriteFile(filepath.Join(stagedDir, controllerruntimeconfig.FileNameBootstrapParams), []byte(bootstrapContent), 0o644)
	c.Assert(err, tc.ErrorIsNil)

	s.PatchValue(&osGetenv, func(key string) string {
		switch key {
		case "SNAP_DATA":
			return snapData
		case "SNAP_COMMON":
			return snapCommon
		}
		return ""
	})

	ic := &initCommand{stagedDir: stagedDir}
	ctx := newTestContext()
	err = ic.Run(ctx)
	c.Assert(err, tc.ErrorIsNil)

	runtimeDst := filepath.Join(snapData, controllerAgentDir, controllerruntimeconfig.Filename)
	resolved, err := controllerruntimeconfig.ReadControllerRuntimeConfig(runtimeDst)
	c.Assert(err, tc.ErrorIsNil)

	// Verify the four snap-path fields are resolved.
	c.Check(resolved.DataDir, tc.Equals, snapData)
	c.Check(resolved.LogDir, tc.Equals, snapCommon+"/var/log/juju")
	c.Check(resolved.SocketDir, tc.Equals, snapCommon+"/sockets")
	c.Check(resolved.SharedAgentDir, tc.Equals, snapCommon+"/agents/controller-0")

	// Verify credential fields are byte-for-byte unchanged.
	c.Check(resolved.CACert, tc.Equals, "ca-cert-pem")
	c.Check(resolved.CAPrivateKey, tc.Equals, "ca-private-key-pem")
	c.Check(resolved.ControllerCert, tc.Equals, "controller-cert-pem")
	c.Check(resolved.ControllerPrivateKey, tc.Equals, "controller-private-key-pem")
	c.Check(resolved.AgentPassword, tc.Equals, "agent-password")
}

// TestTokenInCredentialFieldRejected verifies that the bounded token contract
// rejects any token that appears in a non-path (credential) field.
func (s *InitCommandSuite) TestTokenInCredentialFieldRejected(c *tc.C) {
	stagedDir := c.MkDir()
	snapData := c.MkDir()
	snapCommon := c.MkDir()

	// Craft a config that has a token in the CACert field.
	cfg := controllerruntimeconfig.ControllerRuntimeConfig{
		ControllerID:         "0",
		ControllerUUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		ControllerModelUUID:  "feedface-dead-beef-cafe-c0ffee000000",
		DataDir:              controllerruntimeconfig.TokenSnapData,
		LogDir:               controllerruntimeconfig.TokenSnapCommon + "/var/log/juju",
		SocketDir:            controllerruntimeconfig.TokenSnapCommon + "/sockets",
		SharedAgentDir:       controllerruntimeconfig.TokenSnapCommon + "/agents/controller-0",
		APIPort:              17070,
		AgentPassword:        "agent-password",
		CACert:               "ca-cert-pem with " + controllerruntimeconfig.TokenSnapData + " embedded",
		CAPrivateKey:         "ca-private-key-pem",
		ControllerCert:       "controller-cert-pem",
		ControllerPrivateKey: "controller-private-key-pem",
		APIAddresses:         []string{"10.0.0.1:17070"},
	}
	import_yaml := "controller-id: \"0\"\n" +
		"controller-uuid: deadbeef-0bad-400d-8000-4b1d0d06f00d\n" +
		"controller-model-uuid: feedface-dead-beef-cafe-c0ffee000000\n" +
		"data-dir: \"@SNAP_DATA@\"\n" +
		"log-dir: \"@SNAP_COMMON@/var/log/juju\"\n" +
		"socket-dir: \"@SNAP_COMMON@/sockets\"\n" +
		"shared-agent-dir: \"@SNAP_COMMON@/agents/controller-0\"\n" +
		"api-port: 17070\n" +
		"agent-password: agent-password\n" +
		"ca-cert: \"ca-cert-pem with @SNAP_DATA@ embedded\"\n" +
		"ca-private-key: ca-private-key-pem\n" +
		"controller-cert: controller-cert-pem\n" +
		"controller-private-key: controller-private-key-pem\n" +
		"api-addresses:\n- 10.0.0.1:17070\n"
	_ = cfg

	err := os.WriteFile(filepath.Join(stagedDir, controllerruntimeconfig.Filename), []byte(import_yaml), 0o644)
	c.Assert(err, tc.ErrorIsNil)
	err = os.WriteFile(filepath.Join(stagedDir, controllerruntimeconfig.FileNameBootstrapParams), []byte(`{}`), 0o644)
	c.Assert(err, tc.ErrorIsNil)

	s.PatchValue(&osGetenv, func(key string) string {
		switch key {
		case "SNAP_DATA":
			return snapData
		case "SNAP_COMMON":
			return snapCommon
		}
		return ""
	})

	ic := &initCommand{stagedDir: stagedDir}
	ctx := newTestContext()
	err = ic.Run(ctx)
	c.Check(err, tc.ErrorMatches, `.*token found in non-path field.*`)
}

// TestCredentialLikeStringPreserved verifies that a credential value that
// happens to contain token-like text only triggers the check if it uses the
// exact token constants; other similar strings are preserved unchanged.
func (s *InitCommandSuite) TestCredentialLikeStringPreserved(c *tc.C) {
	// This test uses ResolveStagedControllerRuntimeConfig directly to
	// ensure the bounded token contract preserves a password that contains
	// the token substring only when the field is a non-path field.
	// (A token in a credential field must fail; a non-token value that
	// looks similar must succeed.)
	cfg := controllerruntimeconfig.ControllerRuntimeConfig{
		ControllerID:         "0",
		ControllerUUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		ControllerModelUUID:  "feedface-dead-beef-cafe-c0ffee000000",
		DataDir:              controllerruntimeconfig.TokenSnapData,
		LogDir:               controllerruntimeconfig.TokenSnapCommon + "/var/log/juju",
		SocketDir:            controllerruntimeconfig.TokenSnapCommon + "/sockets",
		SharedAgentDir:       controllerruntimeconfig.TokenSnapCommon + "/agents/controller-0",
		APIPort:              17070,
		AgentPassword:        "passWd!NotAToken",
		CACert:               "ca-cert-pem",
		CAPrivateKey:         "ca-private-key-pem",
		ControllerCert:       "controller-cert-pem",
		ControllerPrivateKey: "controller-private-key-pem",
		APIAddresses:         []string{"10.0.0.1:17070"},
	}
	data, err := controllerruntimeconfig.RenderStagedControllerRuntimeConfig(cfg)
	c.Assert(err, tc.ErrorIsNil)

	snapData := "/fake/snap/data"
	snapCommon := "/fake/snap/common"
	resolved, err := controllerruntimeconfig.ResolveStagedControllerRuntimeConfig(data, snapData, snapCommon)
	c.Assert(err, tc.ErrorIsNil)

	// Credential field is preserved byte-for-byte.
	c.Check(resolved.AgentPassword, tc.Equals, "passWd!NotAToken")
	// Path fields are resolved.
	c.Check(resolved.DataDir, tc.Equals, snapData)
	c.Check(strings.Contains(resolved.LogDir, snapCommon), tc.IsTrue)
}

func (s *InitCommandSuite) TestInitWithExtraArgs(c *tc.C) {
	ic := &initCommand{}
	err := ic.Init([]string{"extra"})
	c.Check(err, tc.ErrorMatches, "unrecognized args.*")
}

func (s *InitCommandSuite) TestCopyStagedFileSrcNotExist(c *tc.C) {
	err := copyStagedFile("/nonexistent/src", "/tmp/dst", 0o600, 0o700)
	c.Check(err, tc.ErrorMatches, `staged file.*does not exist`)
}

func (s *InitCommandSuite) TestCopyStagedFileCreatesParentDirs(c *tc.C) {
	srcDir := c.MkDir()
	src := filepath.Join(srcDir, "src.txt")
	err := os.WriteFile(src, []byte("hello"), 0o644)
	c.Assert(err, tc.ErrorIsNil)

	dstDir := c.MkDir()
	dst := filepath.Join(dstDir, "new", "sub", "dst.txt")

	err = copyStagedFile(src, dst, 0o600, 0o750)
	c.Assert(err, tc.ErrorIsNil)

	// Verify file was created.
	data, err := os.ReadFile(dst)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(data), tc.Equals, "hello")

	// Verify file mode.
	info, err := os.Stat(dst)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.Mode().Perm(), tc.Equals, os.FileMode(0o600))

	// Verify parent directory mode.
	parentInfo, err := os.Stat(filepath.Join(dstDir, "new", "sub"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(parentInfo.Mode().Perm(), tc.Equals, os.FileMode(0o750))
}
