// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"os"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/internal/controllerruntimeconfig"
)

// controllerConfigCommand applies snap-config runtime overrides (currently
// only logging-override) to an existing runtime.conf. It is intended to be
// called by the jujud snap's configure hook.
type controllerConfigCommand struct {
	cmd.CommandBase

	loggingOverride   string
	runtimeConfigPath string
	snapCommon        string
}

// NewControllerConfigCommand returns a new controllerConfigCommand.
func NewControllerConfigCommand() *controllerConfigCommand {
	return &controllerConfigCommand{}
}

func (c *controllerConfigCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "controller-config",
		Purpose: "apply snap-config runtime overrides to runtime.conf",
	})
}

func (c *controllerConfigCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.loggingOverride, "logging-override", "", "logging override value to apply")
	f.StringVar(&c.runtimeConfigPath, "runtime-config-path", "", "path to runtime.conf")
	f.StringVar(&c.snapCommon, "snap-common", "", "path to $SNAP_COMMON directory")
}

func (c *controllerConfigCommand) Init(args []string) error {
	if c.runtimeConfigPath == "" {
		return errors.New("--runtime-config-path is required")
	}
	if c.snapCommon == "" {
		return errors.New("--snap-common is required")
	}
	return cmd.CheckEmpty(args)
}

func (c *controllerConfigCommand) Run(ctx *cmd.Context) error {
	// Validate the supplied keys against the Phase 1 ownership
	// allowlist before touching any on-disk state. This enforces the
	// same ownership boundary at the Go layer regardless of caller.
	vals := map[string]string{
		"logging-override": c.loggingOverride,
	}
	if err := controllerruntimeconfig.ValidateSnapConfigOverlay(vals); err != nil {
		return err
	}

	_, err := os.Stat(c.runtimeConfigPath)
	if os.IsNotExist(err) {
		// runtime.conf does not exist yet: defer the value.
		return controllerruntimeconfig.WriteDeferredLoggingOverride(c.snapCommon, c.loggingOverride)
	}
	if err != nil {
		return errors.Annotatef(err, "checking runtime config %q", c.runtimeConfigPath)
	}

	// If logging-override is explicitly empty (-logging-override ""),
	// clear the field. If the flag was not set (also empty), no-op.
	snapOverlay := controllerruntimeconfig.SnapConfigOverlay{
		LoggingOverride: strings.TrimSpace(c.loggingOverride),
	}
	if err := controllerruntimeconfig.ApplySnapConfigOverlay(c.runtimeConfigPath, snapOverlay); err != nil {
		return errors.Annotate(err, "applying snap config to runtime config")
	}

	_, _ = ctx.Stdout.Write([]byte("applied logging-override\n"))
	return nil
}
