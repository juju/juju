// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/cmd/cmd"
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

	// Create runtime.conf but not bootstrap-params.
	err := os.WriteFile(filepath.Join(stagedDir, "runtime.conf"), []byte("data-dir: /test\n"), 0o644)
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

	runtimeContent := "data-dir: /snap/data\nsocket-dir: /snap/common/sockets\n"
	bootstrapContent := `{"controller-config":{}}`

	err := os.WriteFile(filepath.Join(stagedDir, "runtime.conf"), []byte(runtimeContent), 0o644)
	c.Assert(err, tc.ErrorIsNil)
	err = os.WriteFile(filepath.Join(stagedDir, "bootstrap-params"), []byte(bootstrapContent), 0o644)
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

	// Verify runtime.conf was written to $SNAP_DATA.
	runtimeDst := filepath.Join(snapData, controllerAgentDir, "runtime.conf")
	data, err := os.ReadFile(runtimeDst)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(data), tc.Equals, runtimeContent)

	// Verify runtime.conf file permissions.
	info, err := os.Stat(runtimeDst)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.Mode().Perm(), tc.Equals, os.FileMode(0o600))

	// Verify runtime.conf parent directory permissions.
	parentInfo, err := os.Stat(filepath.Dir(runtimeDst))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(parentInfo.Mode().Perm(), tc.Equals, os.FileMode(0o700))

	// Verify bootstrap-params was written to $SNAP_COMMON.
	bootstrapDst := filepath.Join(snapCommon, "bootstrap-params")
	data, err = os.ReadFile(bootstrapDst)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(data), tc.Equals, bootstrapContent)

	// Verify bootstrap-params file permissions.
	info, err = os.Stat(bootstrapDst)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.Mode().Perm(), tc.Equals, os.FileMode(0o600))
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
