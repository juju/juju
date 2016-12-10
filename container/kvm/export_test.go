// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build linux
// +build amd64 arm64 ppc64el

package kvm

import "strings"

// This file exports internal package implementations so that tests
// can utilize them to mock behavior.

var (
	KVMPath = &kvmPath

	// Used to export the parameters used to call Start on the KVM Container
	TestStartParams = &startParams
)

func MakeTestableCreateMachineParams(params *CreateMachineParams, pathfinder func(string) (string, error), runCmd runFunc) {
	params.pathfinder = pathfinder
	params.runCmd = runCmd
	return
}

func NewEmptyKvmContainer() *kvmContainer {
	return &kvmContainer{}
}

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

func (s *runStub) Run(cmd string, args ...string) (string, error) {
	call := []string{cmd}
	call = append(call, args...)
	s.calls = append(s.calls, strings.Join(call, " "))
	if s.err != nil {
		return s.err.Error(), s.err
	}
	return s.output, nil
}

func (s *runStub) Calls() []string {
	return s.calls
}
