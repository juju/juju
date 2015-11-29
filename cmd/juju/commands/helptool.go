// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/network"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

// dummyHookContext implements jujuc.Context,
// as expected by jujuc.NewCommand.
type dummyHookContext struct{ jujuc.Context }

func (dummyHookContext) AddMetrics(_, _ string, _ time.Time) error {
	return nil
}
func (dummyHookContext) UnitName() string {
	return ""
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
	return &cmd.Info{
		Name:    "help-tool",
		Args:    "[tool]",
		Purpose: "show help on a juju charm tool",
	}
}

func (t *helpToolCommand) Init(args []string) error {
	tool, err := cmd.ZeroOrOneArgs(args)
	if err == nil {
		t.tool = tool
	}
	return err
}

func (c *helpToolCommand) Run(ctx *cmd.Context) error {
	var hookctx dummyHookContext
	if c.tool == "" {
		// Ripped from SuperCommand. We could Run() a SuperCommand
		// with "help commands", but then the implicit "help" command
		// shows up.
		names := jujuc.CommandNames()
		cmds := make([]cmd.Command, 0, len(names))
		longest := 0
		for _, name := range names {
			if c, err := jujuc.NewCommand(hookctx, name); err == nil {
				if len(name) > longest {
					longest = len(name)
				}
				cmds = append(cmds, c)
			}
		}
		for _, c := range cmds {
			info := c.Info()
			fmt.Fprintf(ctx.Stdout, "%-*s  %s\n", longest, info.Name, info.Purpose)
		}
	} else {
		c, err := jujuc.NewCommand(hookctx, c.tool)
		if err != nil {
			return err
		}
		info := c.Info()
		f := gnuflag.NewFlagSet(info.Name, gnuflag.ContinueOnError)
		c.SetFlags(f)
		ctx.Stdout.Write(info.Help(f))
	}
	return nil
}
