// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import (
	"strings"

	"github.com/juju/juju/environs/instances"
)

// This file exports internal package implementations so that tests
// can utilize them to mock behavior.

var KVMPath = &kvmPath

// MakeCreateMachineParamsTestable adds test values to non exported values on
// CreateMachineParams.
func MakeCreateMachineParamsTestable(params *CreateMachineParams, pathfinder pathfinderFunc, runCmd runFunc, arch string) {
	params.findPath = pathfinder
	params.runCmd = runCmd
	params.runCmdAsRoot = runCmd
	params.arch = arch
	return
}

// NewEmptyKvmContainer returns an empty kvmContainer for testing.
func NewEmptyKvmContainer() *kvmContainer {
	return &kvmContainer{}
}

// NewTestContainer returns a new container for testing.
func NewTestContainer(name string, runCmd runFunc, pathfinder pathfinderFunc) *kvmContainer {
	return &kvmContainer{name: name, runCmd: runCmd, pathfinder: pathfinder}
}

// ContainerFromInstance extracts the inner container from input instance,
// so we can access it for test assertions.
func ContainerFromInstance(inst instances.Instance) Container {
	kvm := inst.(*kvmInstance)
	return kvm.container
}

// NewRunStub is a stub to fake shelling out to os.Exec or utils.RunCommand.
func NewRunStub(output string, err error) *runStub {
	return &runStub{output: output, err: err}
}

type runStub struct {
	output string
	err    error
	calls  []string
}

// Run fakes running commands, instead recording calls made for use in testing.
func (s *runStub) Run(dir, cmd string, args ...string) (string, error) {
	call := []string{dir, cmd}
	call = append(call, args...)
	s.calls = append(s.calls, strings.Join(call, " "))
	if s.err != nil {
		return s.err.Error(), s.err
	}
	return s.output, nil
}

// Calls returns the calls made on a runStub.
func (s *runStub) Calls() []string {
	return s.calls
}
