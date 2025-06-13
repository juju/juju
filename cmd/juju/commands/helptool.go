// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"time"

	"github.com/juju/charm/v8"
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

// dummyHookContext implements hooks.Context,
// as expected by hooks.NewCommand.
type dummyHookContext struct{ jujuc.Context }

func (dummyHookContext) AddMetrics(_, _ string, _ time.Time) error {
	return nil
}
func (dummyHookContext) UnitName() string {
	return ""
}
func (dummyHookContext) SetPodSpec(specYaml string) error {
	return nil
}
func (dummyHookContext) GetPodSpec() (string, error) {
	return "", nil
}
func (dummyHookContext) SetRawK8sSpec(specYaml string) error {
	return nil
}
func (dummyHookContext) GetRawK8sSpec() (string, error) {
	return "", nil
}
func (dummyHookContext) PublicAddress() (string, error) {
	return "", errors.NotFoundf("PublicAddress")
}
func (dummyHookContext) PrivateAddress() (string, error) {
	return "", errors.NotFoundf("PrivateAddress")
}
func (dummyHookContext) AvailabilityZone() (string, error) {
	return "", errors.NotFoundf("AvailabilityZone")
}
func (dummyHookContext) OpenPort(protocol string, port int) error {
	return nil
}
func (dummyHookContext) ClosePort(protocol string, port int) error {
	return nil
}
func (dummyHookContext) OpenedPorts() []network.PortRange {
	return nil
}
func (dummyHookContext) ConfigSettings() (charm.Settings, error) {
	return charm.NewConfig().DefaultSettings(), nil
}
func (dummyHookContext) HookRelation() (jujuc.ContextRelation, error) {
	return nil, errors.NotFoundf("HookRelation")
}
func (dummyHookContext) RemoteUnitName() (string, error) {
	return "", errors.NotFoundf("RemoteUnitName")
}
func (dummyHookContext) RemoteApplicationName() (string, error) {
	return "", errors.NotFoundf("RemoteApplicationName")
}
func (dummyHookContext) Relation(id int) (jujuc.ContextRelation, error) {
	return nil, errors.NotFoundf("Relation")
}
func (dummyHookContext) RelationIds() ([]int, error) {
	return []int{}, errors.NotFoundf("RelationIds")
}

func (dummyHookContext) RequestReboot(prio jujuc.RebootPriority) error {
	return nil
}

func (dummyHookContext) HookStorageInstance() (*storage.StorageInstance, error) {
	return nil, errors.NotFoundf("HookStorageInstance")
}

func (dummyHookContext) HookStorage() (jujuc.ContextStorageAttachment, error) {
	return nil, errors.NotFoundf("HookStorage")
}

func (dummyHookContext) StorageInstance(id string) (*storage.StorageInstance, error) {
	return nil, errors.NotFoundf("StorageInstance")
}

func (dummyHookContext) UnitStatus() (*jujuc.StatusInfo, error) {
	return &jujuc.StatusInfo{}, nil
}

func (dummyHookContext) SetStatus(jujuc.StatusInfo) error {
	return nil
}

func newHelpToolCommand() cmd.Command {
	return &helpToolCommand{}
}

type helpToolCommand struct {
	cmd.CommandBase
	tool string
}

func (t *helpToolCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "hook-tool",
		Args:    "[tool]",
		Purpose: "Show help on a Juju charm hook tool.",
		Doc:     helpToolDoc,
		Aliases: []string{
			"help-tool", // TODO (anastasimac 2017-11-1) This should be removed in Juju 3.
			"hook-tools"},
	})
}

func (t *helpToolCommand) Init(args []string) error {
	tool, err := cmd.ZeroOrOneArgs(args)
	if err == nil {
		t.tool = tool
	}
	return err
}

func (c *helpToolCommand) Run(ctx *cmd.Context) error {
	if c.tool == "" {
		fmt.Fprint(ctx.Stdout, listHookTools())
	} else {
		c, err := jujuc.NewCommand(dummyHookContext{}, c.tool)
		if err != nil {
			return err
		}
		info := c.Info()
		f := gnuflag.NewFlagSetWithFlagKnownAs(info.Name, gnuflag.ContinueOnError, cmd.FlagAlias(c, "option"))
		c.SetFlags(f)
		_, _ = ctx.Stdout.Write(info.Help(f))
	}
	return nil
}

var helpToolDoc = fmt.Sprintf(`
Juju charms can access a series of built-in helpers called 'hook-tools'.
These are useful for the charm to be able to inspect its running environment.
Currently available charm hook tools are:

%v
Examples:

    For help on a specific tool, supply the name of that tool, for example:

        juju hook-tool unit-get

`, listHookTools())

func listHookTools() string {
	all := ""
	// Ripped from SuperCommand. We could Run() a SuperCommand
	// with "help commands", but then the implicit "help" command
	// shows up.
	names := jujuc.CommandNames()
	cmds := []cmd.Command{}
	longest := 0
	for _, name := range names {
		if c, err := jujuc.NewCommand(dummyHookContext{}, name); err == nil {
			// On Windows name has a '.exe' suffix, while Info().Name does not
			name := c.Info().Name
			if len(name) > longest {
				longest = len(name)
			}
			cmds = append(cmds, c)
		}
	}
	for _, c := range cmds {
		info := c.Info()
		all += fmt.Sprintf("    %-*s  %s\n", longest, info.Name, info.Purpose)
	}
	return all
}
