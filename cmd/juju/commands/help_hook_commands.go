// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"time"

	"github.com/juju/charm/v12"
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
	"github.com/juju/juju/storage"
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

func newhelpHookCmdsCommand() cmd.Command {
	return &helpHookCmdsCommand{}
}

type helpHookCmdsCommand struct {
	cmd.CommandBase
	hookCmd string
}

func (t *helpHookCmdsCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "help-hook-commands",
		Args:     "[hook]",
		Purpose:  "Show help on a Juju charm hook command.",
		Doc:      helpHookCmdsDoc,
		Examples: helpHookCmdsExamples,
		SeeAlso:  []string{"help", "help-action-commands"},
	})
}

func (t *helpHookCmdsCommand) Init(args []string) error {
	hookCmd, err := cmd.ZeroOrOneArgs(args)
	if err == nil {
		t.hookCmd = hookCmd
	}
	return err
}

func (c *helpHookCmdsCommand) Run(ctx *cmd.Context) error {
	if c.hookCmd == "" {
		fmt.Fprint(ctx.Stdout, listHelpHookCmds())
	} else {
		c, err := jujuc.NewHookCommandForHelp(dummyHookContext{}, c.hookCmd)
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

var helpHookCmdsDoc = fmt.Sprintf(`
Juju charms have access to a set of built-in helpers known as 'hook-commands,'
which allow them to inspect their runtime environment.
The currently available charm hook commands include:

%v
`, listHelpHookCmds())

const helpHookCmdsExamples = `
For help on a specific hook command, supply the name of that hook command, for example:

        juju help-hook-commands unit-get
`

func listHelpHookCmds() string {
	all := ""
	// Ripped from SuperCommand. We could Run() a SuperCommand
	// with "help commands", but then the implicit "help" command
	// shows up.
	names := jujuc.HookCommandNames()
	cmds := []cmd.Command{}
	longest := 0
	for _, name := range names {
		if c, err := jujuc.NewHookCommandForHelp(dummyHookContext{}, name); err == nil {
			// On Windows name has a '.exe' suffix, while Info().Name does not
			name := c.Info().Name
			if len(name) > longest {
				// Left-aligns the command purpose to match the longest command name length.
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
