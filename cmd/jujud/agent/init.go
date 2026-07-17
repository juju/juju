// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/utils/v4"
	"gopkg.in/yaml.v2"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
	runtimeconf "github.com/juju/juju/internal/controllerruntimeconfig"
)

// initCommand delivers snap-private files into the controller snap's
// $SNAP_DATA and $SNAP_COMMON directories. When the controller runs under
// strict confinement, cloud-init must not write directly into the snap
// directory tree. Instead, cloud-init stages runtime.conf and bootstrap-params
// in a temporary directory, then invokes this command via "snap run jujud.init
// <staged-dir>". The command runs inside the snap's context, so
// $SNAP_DATA and $SNAP_COMMON are resolved by snapd to the correct
// revision-specific directories.
//
// For runtime.conf the command parses the staged file, resolves the four
// documented snap-path tokens (@SNAP_DATA@ and @SNAP_COMMON@) using the
// process's SNAP_DATA and SNAP_COMMON environment values, validates the
// result, and atomically writes the final file with 0600 permissions. For
// bootstrap-params the file is copied byte-for-byte with 0600 permissions.
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

func (c *initCommand) Init(args []string) error {
	if len(args) != 1 {
		return errors.New("expected exactly one argument: <staged-dir>")
	}
	c.stagedDir = args[0]
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

	runtimeSrc := filepath.Join(c.stagedDir, runtimeconf.Filename)
	runtimeData, err := os.ReadFile(runtimeSrc)
	if err != nil {
		if os.IsNotExist(err) {
			return errors.Errorf("staged file %q does not exist", runtimeSrc)
		}
		return errors.Annotatef(err, "reading staged runtime.conf %q", runtimeSrc)
	}
	resolvedCfg, err := func() (runtimeconf.ControllerRuntimeConfig, error) {
		var staged runtimeconf.StagedControllerRuntimeConfig
		if err := yaml.Unmarshal(runtimeData, &staged); err != nil {
			return runtimeconf.ControllerRuntimeConfig{}, errors.Annotate(err, "parsing staged runtime.conf")
		}
		return runtimeconf.ResolveStagedControllerRuntimeConfig(staged, snapData, snapCommon)
	}()
	if err != nil {
		return errors.Annotate(err, "resolving staged runtime.conf")
	}
	runtimeDst := filepath.Join(snapData, controllerAgentDir, runtimeconf.Filename)
	if err := runtimeconf.WriteControllerRuntimeConfig(runtimeDst, resolvedCfg); err != nil {
		return errors.Annotate(err, "writing resolved runtime.conf")
	}
	_, _ = fmt.Fprintf(ctx.Stdout, "Wrote %s\n", runtimeDst)

	// Copy bootstrap-params from staged dir to $SNAP_COMMON byte-for-byte.
	// bootstrap-params is always present during the initial bootstrap
	// cycle that triggers this init command. It is not re-staged after
	// bootstrap completes, so the init command is not designed for
	// re-execution outside the cloud-init delivery flow.

	bootstrapSrc := filepath.Join(c.stagedDir, runtimeconf.FileNameBootstrapParams)
	bootstrapDst := filepath.Join(snapCommon, runtimeconf.FileNameBootstrapParams)
	if err := copyStagedFile(bootstrapSrc, bootstrapDst, 0o600, 0o755); err != nil {
		return errors.Annotate(err, "copying bootstrap-params")
	}
	_, _ = fmt.Fprintf(ctx.Stdout, "Wrote %s\n", bootstrapDst)

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
