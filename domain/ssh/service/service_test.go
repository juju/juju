// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/tc"
	gossh "golang.org/x/crypto/ssh"

	coremachine "github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/virtualhostname"
	sshservice "github.com/juju/juju/domain/ssh/service"
	"github.com/juju/juju/internal/errors"
)

type serviceSuite struct{}

func TestServiceSuite(t *stdtesting.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) TestSSHServerHostKeyReturnsExisting(c *tc.C) {
	controllerState := &stubControllerState{
		key:   testPrivateKey,
		found: true,
	}

	svc := sshservice.NewService(controllerState, nil)
	key, err := svc.SSHServerHostKey(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(key, tc.Equals, testPrivateKey)
}

func (s *serviceSuite) TestSSHServerHostKeyErrorsWhenMissing(c *tc.C) {
	svc := sshservice.NewService(&stubControllerState{}, nil)

	key, err := svc.SSHServerHostKey(c.Context())
	c.Check(key, tc.Equals, "")
	c.Assert(err, tc.ErrorMatches, `controller SSH server host key not found`)
}

func (s *serviceSuite) TestMachineVirtualHostKeyGeneratesMissing(c *tc.C) {
	modelUUID := coremodel.UUID(testModelUUID)
	state := newStubModelState()
	state.machineExists["1"] = true

	svc := sshservice.NewService(&stubControllerState{}, sshservice.ModelStateGetterFunc(func(_ coremodel.UUID) sshservice.ModelState {
		return state
	}))

	key, err := svc.MachineVirtualHostKey(c.Context(), modelUUID, coremachine.Name("1"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(state.machineSetCalls, tc.Equals, 1)
	c.Check(state.machineKeys["1"], tc.Equals, key)
	assertPrivateKey(c, key)
}

func (s *serviceSuite) TestUnitVirtualHostKeyUsesBackingMachine(c *tc.C) {
	modelUUID := coremodel.UUID(testModelUUID)
	state := newStubModelState()
	state.machineExists["1"] = true
	state.machineKeys["1"] = testPrivateKey
	state.unitMachines["postgresql/0"] = "1"

	svc := sshservice.NewService(&stubControllerState{}, sshservice.ModelStateGetterFunc(func(_ coremodel.UUID) sshservice.ModelState {
		return state
	}))

	key, err := svc.UnitVirtualHostKey(c.Context(), modelUUID, coreunit.Name("postgresql/0"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(key, tc.Equals, testPrivateKey)
	c.Check(state.unitSetCalls, tc.Equals, 0)
	c.Check(state.machineSetCalls, tc.Equals, 0)
}

func (s *serviceSuite) TestUnitVirtualHostKeyGeneratesMissingForCAAS(c *tc.C) {
	modelUUID := coremodel.UUID(testModelUUID)
	state := newStubModelState()
	state.unitExists["postgresql/0"] = true

	svc := sshservice.NewService(&stubControllerState{}, sshservice.ModelStateGetterFunc(func(_ coremodel.UUID) sshservice.ModelState {
		return state
	}))

	key, err := svc.UnitVirtualHostKey(c.Context(), modelUUID, coreunit.Name("postgresql/0"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(state.unitSetCalls, tc.Equals, 1)
	c.Check(state.unitKeys["postgresql/0"], tc.Equals, key)
	assertPrivateKey(c, key)
}

func (s *serviceSuite) TestVirtualHostKeyFromMachineInfo(c *tc.C) {
	state := newStubModelState()
	state.machineExists["1"] = true
	state.machineKeys["1"] = testPrivateKey

	svc := sshservice.NewService(&stubControllerState{}, sshservice.ModelStateGetterFunc(func(_ coremodel.UUID) sshservice.ModelState {
		return state
	}))

	info, err := virtualhostname.NewInfoMachineTarget(testModelUUID, "1")
	c.Assert(err, tc.ErrorIsNil)

	key, err := svc.VirtualHostKey(c.Context(), info)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(key, tc.Equals, testPrivateKey)
}

type stubControllerState struct {
	key    string
	found  bool
	getErr error
}

func (s *stubControllerState) GetSSHServerHostKey(_ context.Context) (string, bool, error) {
	return s.key, s.found, s.getErr
}

type stubModelState struct {
	machineExists   map[string]bool
	machineKeys     map[string]string
	unitExists      map[string]bool
	unitKeys        map[string]string
	unitMachines    map[string]string
	machineSetCalls int
	unitSetCalls    int
}

func newStubModelState() *stubModelState {
	return &stubModelState{
		machineExists: make(map[string]bool),
		machineKeys:   make(map[string]string),
		unitExists:    make(map[string]bool),
		unitKeys:      make(map[string]string),
		unitMachines:  make(map[string]string),
	}
}

func (s *stubModelState) GetMachineVirtualHostKeyByMachineName(_ context.Context, machineName string) (string, bool, error) {
	if !s.machineExists[machineName] {
		return "", false, errors.Errorf("machine %q not found", machineName)
	}
	key, found := s.machineKeys[machineName]
	return key, found, nil
}

func (s *stubModelState) SetMachineVirtualHostKeyByMachineName(_ context.Context, machineName, key string) error {
	if !s.machineExists[machineName] {
		return errors.Errorf("machine %q not found", machineName)
	}
	s.machineKeys[machineName] = key
	s.machineSetCalls++
	return nil
}

func (s *stubModelState) GetUnitVirtualHostKeyByUnitName(_ context.Context, unitName string) (string, bool, error) {
	if !s.unitExists[unitName] {
		return "", false, errors.Errorf("unit %q not found", unitName)
	}
	key, found := s.unitKeys[unitName]
	return key, found, nil
}

func (s *stubModelState) SetUnitVirtualHostKeyByUnitName(_ context.Context, unitName, key string) error {
	if !s.unitExists[unitName] {
		return errors.Errorf("unit %q not found", unitName)
	}
	s.unitKeys[unitName] = key
	s.unitSetCalls++
	return nil
}

func (s *stubModelState) GetMachineNameForUnit(_ context.Context, unitName string) (string, bool, error) {
	if !s.unitExists[unitName] && s.unitMachines[unitName] == "" {
		return "", false, errors.Errorf("unit %q not found", unitName)
	}
	machineName, found := s.unitMachines[unitName]
	return machineName, found, nil
}

func assertPrivateKey(c *tc.C, key string) {
	_, err := gossh.ParsePrivateKey([]byte(key))
	c.Assert(err, tc.ErrorIsNil)
}

const (
	testModelUUID  = "8419cd78-4993-4c3a-928e-c646226beeee"
	testPrivateKey = "-----BEGIN OPENSSH PRIVATE KEY-----\n" +
		"b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtz\n" +
		"c2gtZWQyNTUxOQAAACBT8UidoqUmpUFFCGEhZhHWGE7VHoJY7LZ7yXzuWlSVYAAA\n" +
		"AIiZq0wRmatMEQAAAAtzc2gtZWQyNTUxOQAAACBT8UidoqUmpUFFCGEhZhHWGE7V\n" +
		"HoJY7LZ7yXzuWlSVYAAAAEBYRsJTytYJUidtOuv3s3tdjyDA+4TSdCz9+hFKjyqz\n" +
		"v1PxSJ2ipSalQUUIYSFmEdYYTtUegljstnvJfO5aVJVgAAAAAAECAwQF\n" +
		"-----END OPENSSH PRIVATE KEY-----\n"
)
