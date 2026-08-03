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
	flagSet           *gnuflag.FlagSet
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
	c.flagSet = f
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
	vals := map[string]string{
		"logging-override": c.loggingOverride,
	}
	if err := controllerruntimeconfig.ValidateSnapConfigOverlay(vals); err != nil {
		return err
	}

	_, err := os.Stat(c.runtimeConfigPath)
	if os.IsNotExist(err) {
		return controllerruntimeconfig.WriteDeferredLoggingOverride(c.snapCommon, c.loggingOverride)
	}
	if err != nil {
		return errors.Annotatef(err, "checking runtime config %q", c.runtimeConfigPath)
	}

	// logging-override maps directly to the flag value. When the flag
	// is not set the empty default is indistinguishable from an
	// explicit clear (--logging-override ""). Use gnuflag.Visit to
	// detect whether the operator actually passed the flag so that
	// an invocation without --logging-override does not accidentally
	// wipe a previously-applied value.
	loggingOverrideSet := false
	if c.flagSet != nil {
		c.flagSet.Visit(func(f *gnuflag.Flag) {
			if f.Name == "logging-override" {
				loggingOverrideSet = true
			}
		})
	}
	if c.flagSet != nil && !loggingOverrideSet {
		return nil
	}

	snapOverlay := controllerruntimeconfig.SnapConfigOverlay{
		LoggingOverride: strings.TrimSpace(c.loggingOverride),
	}
	if err := controllerruntimeconfig.ApplySnapConfigOverlay(c.runtimeConfigPath, snapOverlay); err != nil {
		return errors.Annotate(err, "applying snap config to runtime config")
	}
	if err := controllerruntimeconfig.WriteDeferredLoggingOverride(c.snapCommon, strings.TrimSpace(c.loggingOverride)); err != nil {
		return errors.Annotate(err, "writing state file")
	}

	_, _ = ctx.Stdout.Write([]byte("applied logging-override\n"))
	return nil
}
