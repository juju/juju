// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"os"
	"slices"
	"time"

	"github.com/juju/charm/v12"
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
	"github.com/juju/juju/storage"
)

// These commands are deprecated, we don't want to document them.
var ignoreCommands = []string{"k8s-raw-get", "k8s-raw-set", "k8s-spec-get",
	"k8s-spec-set", "pod-spec-get", "pod-spec-set"}

// This script generates Markdown documentation for the hook tools.
// We want to use the same `juju documentation` command to generate docs for
// the hook tools, to ensure the hook tool docs are consistent with the command
// docs. However, there is no super-command for the hook tools like there is
// with juju commands. Hence, this script creates such a super-command, and
// runs the embedded 'documentation' command to generate the docs.
func main() {
	jujucSuperCmd := cmd.NewSuperCommand(cmd.SuperCommandParams{})
	for _, name := range jujuc.HookCommandNames() {
		if slices.Contains(ignoreCommands, name) {
			continue
		}
		hookTool, err := jujuc.NewHookCommand(dummyHookContext{}, name)
		check(err)
		jujucSuperCmd.Register(hookTool)
	}
	for _, name := range jujuc.ActionCommandNames() {
		if slices.Contains(ignoreCommands, name) {
			continue
		}
		actionTool, err := jujuc.NewActionCommand(dummyHookContext{}, name)
		check(err)
		jujucSuperCmd.Register(actionTool)
	}

	if len(os.Args) < 2 {
		panic("destination directory must be provided")
	}
	dest := os.Args[1]

	jujucSuperCmd.SetFlags(&gnuflag.FlagSet{})
	err := jujucSuperCmd.Init([]string{"documentation", "--split", "--no-index", "--out", dest})
	check(err)
	err = jujucSuperCmd.Run(&cmd.Context{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	})
	check(err)

}

// dummyHookContext implements hooks.Context, as expected by hooks.NewCommand.
// Copied from cmd/juju/commands/helptool.go
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

// UTILITY FUNCTIONS

// check panics if the provided error is not nil.
func check(err error) {
	if err != nil {
		panic(err)
	}
}
