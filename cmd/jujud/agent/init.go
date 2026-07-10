// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/utils/v4"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
)

// initCommand delivers snap-private files into the controller snap's
// $SNAP_DATA and $SNAP_COMMON directories. When the controller runs under
// strict confinement, cloud-init must not write directly into the snap
// directory tree. Instead, cloud-init stages runtime.conf and bootstrap-params
// in a temporary directory, then invokes this command via "snap run jujud.init
// --staged-dir <path>". The command runs inside the snap's context, so
// $SNAP_DATA and $SNAP_COMMON are resolved by snapd to the correct
// revision-specific directories. It copies the staged files to their
// snap-private destinations with correct permissions (0600 for files,
// 0700/0755 for parent dirs).
type initCommand struct {
	cmd.CommandBase
	stagedDir string
}

// NewInitCommand returns a new initCommand.
func NewInitCommand() *initCommand {
	return &initCommand{}
}

func (c *initCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "init",
		Purpose: "initialize snap-private files from staged input",
	})
}

func (c *initCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.stagedDir, "staged-dir", "", "directory containing staged input files")
}

func (c *initCommand) Init(args []string) error {
	if err := cmd.CheckEmpty(args); err != nil {
		return err
	}
	if c.stagedDir == "" {
		return errors.New("--staged-dir is required")
	}
	info, err := os.Stat(c.stagedDir)
	if err != nil {
		return errors.Annotatef(err, "cannot access staged directory %q", c.stagedDir)
	}
	if !info.IsDir() {
		return errors.Errorf("%q is not a directory", c.stagedDir)
	}
	return nil
}

const (
	controllerAgentDir = "agents/controller-0"
)

var osGetenv = os.Getenv

func (c *initCommand) Run(ctx *cmd.Context) error {
	snapData := osGetenv("SNAP_DATA")
	if snapData == "" {
		return errors.New("SNAP_DATA is not set")
	}

	snapCommon := osGetenv("SNAP_COMMON")
	if snapCommon == "" {
		return errors.New("SNAP_COMMON is not set")
	}

	// Copy runtime.conf from staged dir to $SNAP_DATA.
	runtimeSrc := filepath.Join(c.stagedDir, "runtime.conf")
	runtimeDst := filepath.Join(snapData, controllerAgentDir, "runtime.conf")
	if err := copyStagedFile(runtimeSrc, runtimeDst, 0o600, 0o700); err != nil {
		return errors.Annotate(err, "copying runtime.conf")
	}
	fmt.Fprintf(ctx.Stdout, "Wrote %s\n", runtimeDst)

	// Copy bootstrap-params from staged dir to $SNAP_COMMON.
	bootstrapSrc := filepath.Join(c.stagedDir, "bootstrap-params")
	bootstrapDst := filepath.Join(snapCommon, "bootstrap-params")
	if err := copyStagedFile(bootstrapSrc, bootstrapDst, 0o600, 0o755); err != nil {
		return errors.Annotate(err, "copying bootstrap-params")
	}
	fmt.Fprintf(ctx.Stdout, "Wrote %s\n", bootstrapDst)

	return nil
}

func copyStagedFile(src, dst string, fileMode, dirMode os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		if os.IsNotExist(err) {
			return errors.Errorf("staged file %q does not exist", src)
		}
		return errors.Annotatef(err, "reading staged file %q", src)
	}

	parentDir := filepath.Dir(dst)
	if err := os.MkdirAll(parentDir, dirMode); err != nil {
		return errors.Annotatef(err, "creating directory %q", parentDir)
	}

	if err := utils.AtomicWriteFile(dst, data, fileMode); err != nil {
		return errors.Annotatef(err, "writing %q", dst)
	}

	return nil
}
