// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"
	"os"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/internal/controllerruntimeconfig"
)

// NewGetControllerConfigCommand returns a command that reads the current
// logging-override value, falling back to the deferred state file when
// runtime.conf does not exist.
func NewGetControllerConfigCommand() *getControllerConfigCommand {
	return &getControllerConfigCommand{}
}

// NewSetControllerConfigCommand returns a command that applies a
// logging-override value to runtime.conf (or defers it if runtime.conf
// does not exist). It is intended to be called by the jujud snap's
// configure hook after the daemon has been stopped.
func NewSetControllerConfigCommand() *setControllerConfigCommand {
	return &setControllerConfigCommand{}
}

// getControllerConfigCommand reads the current logging-override from
// runtime.conf, falling back to the deferred state file when runtime.conf
// does not exist. It prints the value to stdout.
type getControllerConfigCommand struct {
	cmd.CommandBase

	runtimeConfigPath string
	snapCommon        string
}

func (c *getControllerConfigCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "get-controller-config",
		Purpose: "read the current logging-override value",
	})
}

func (c *getControllerConfigCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.runtimeConfigPath, "runtime-config-path", "", "path to runtime.conf")
	f.StringVar(&c.snapCommon, "snap-common", "", "path to $SNAP_COMMON directory")
}

func (c *getControllerConfigCommand) Init(args []string) error {
	if c.runtimeConfigPath == "" {
		return errors.New("--runtime-config-path is required")
	}
	if c.snapCommon == "" {
		return errors.New("--snap-common is required")
	}
	return cmd.CheckEmpty(args)
}

func (c *getControllerConfigCommand) Run(ctx *cmd.Context) error {
	_, err := os.Stat(c.runtimeConfigPath)
	if os.IsNotExist(err) {
		val, err := controllerruntimeconfig.ReadDeferredLoggingOverride(c.snapCommon)
		if err != nil {
			return errors.Trace(err)
		}
		_, _ = fmt.Fprintf(ctx.Stdout, "%s\n", val)
		return nil
	}
	if err != nil {
		return errors.Annotatef(err, "checking runtime config %q", c.runtimeConfigPath)
	}

	cfg, err := controllerruntimeconfig.ReadControllerRuntimeConfig(c.runtimeConfigPath)
	if err != nil {
		return errors.Annotatef(err, "reading controller runtime config %q", c.runtimeConfigPath)
	}
	_, _ = fmt.Fprintf(ctx.Stdout, "%s\n", cfg.LoggingOverride)
	return nil
}

// setControllerConfigCommand applies a logging-override value to
// runtime.conf. When runtime.conf does not exist it defers the value to
// the state file for jujud.init to apply once the file is staged. It is
// called by the snap configure hook after the daemon has been stopped.
type setControllerConfigCommand struct {
	cmd.CommandBase

	loggingOverride   string
	runtimeConfigPath string
	snapCommon        string
}

func (c *setControllerConfigCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "set-controller-config",
		Purpose: "apply snap-config runtime overrides to runtime.conf",
	})
}

func (c *setControllerConfigCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.loggingOverride, "logging-override", "", "logging override value to apply")
	f.StringVar(&c.runtimeConfigPath, "runtime-config-path", "", "path to runtime.conf")
	f.StringVar(&c.snapCommon, "snap-common", "", "path to $SNAP_COMMON directory")
}

func (c *setControllerConfigCommand) Init(args []string) error {
	if c.runtimeConfigPath == "" {
		return errors.New("--runtime-config-path is required")
	}
	if c.snapCommon == "" {
		return errors.New("--snap-common is required")
	}
	return cmd.CheckEmpty(args)
}

func (c *setControllerConfigCommand) Run(ctx *cmd.Context) error {
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
