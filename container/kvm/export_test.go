// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import "strings"

// This file exports internal package implementations so that tests
// can utilize them to mock behavior.

var (
	// KVMPath is exported for use in tests.
	KVMPath = &kvmPath

	// Used to export the parameters used to call Start on the KVM Container
	TestStartParams = &startParams
)

// MakeCreateMachineParamsTestable adds test values to non exported values on
// CreateMachineParams.
func MakeCreateMachineParamsTestable(params *CreateMachineParams, pathfinder func(string) (string, error), runCmd runFunc, arch string) {
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
func NewTestContainer(name string, runCmd runFunc, pathfinder func(string) (string, error)) *kvmContainer {
	return &kvmContainer{name: name, runCmd: runCmd, pathfinder: pathfinder}
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
func (s *runStub) Run(cmd string, args ...string) (string, error) {
	call := []string{cmd}
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
