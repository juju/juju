// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/tc"
	gossh "golang.org/x/crypto/ssh"

	coremachine "github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/virtualhostname"
	modelsshservice "github.com/juju/juju/domain/ssh/service/model"
	"github.com/juju/juju/internal/errors"
)

type serviceSuite struct{}

func TestServiceSuite(t *stdtesting.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) TestMachineVirtualHostKeyGeneratesMissing(c *tc.C) {
	modelUUID := coremodel.UUID(testModelUUID)
	state := newStubModelState()
	state.machineExists["1"] = true

	svc := modelsshservice.NewService(modelUUID, state)

	key, err := svc.MachineVirtualHostKey(c.Context(), coremachine.Name("1"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(state.machineEnsureCalls, tc.Equals, 1)
	c.Check(state.machineKeys["1"], tc.Equals, key)
	assertPrivateKey(c, key)
}

func (s *serviceSuite) TestUnitVirtualHostKeyUsesBackingMachine(c *tc.C) {
	modelUUID := coremodel.UUID(testModelUUID)
	state := newStubModelState()
	state.machineExists["1"] = true
	state.machineKeys["1"] = testPrivateKey
	state.unitMachines["postgresql/0"] = "1"

	svc := modelsshservice.NewService(modelUUID, state)

	key, err := svc.UnitVirtualHostKey(c.Context(), coreunit.Name("postgresql/0"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(key, tc.Equals, testPrivateKey)
	c.Check(state.unitEnsureCalls, tc.Equals, 0)
	c.Check(state.machineEnsureCalls, tc.Equals, 0)
}

func (s *serviceSuite) TestUnitVirtualHostKeyGeneratesMissingForCAAS(c *tc.C) {
	modelUUID := coremodel.UUID(testModelUUID)
	state := newStubModelState()
	state.unitExists["postgresql/0"] = true

	svc := modelsshservice.NewService(modelUUID, state)

	key, err := svc.UnitVirtualHostKey(c.Context(), coreunit.Name("postgresql/0"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(state.unitEnsureCalls, tc.Equals, 1)
	c.Check(state.unitKeys["postgresql/0"], tc.Equals, key)
	assertPrivateKey(c, key)
}

func (s *serviceSuite) TestMachineVirtualHostKeyReturnsExistingAfterConcurrentInsert(c *tc.C) {
	modelUUID := coremodel.UUID(testModelUUID)
	state := newStubModelState()
	state.machineExists["1"] = true
	state.machineEnsureKeys["1"] = testPrivateKey

	svc := modelsshservice.NewService(modelUUID, state)

	key, err := svc.MachineVirtualHostKey(c.Context(), coremachine.Name("1"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(key, tc.Equals, testPrivateKey)
	c.Check(state.machineEnsureCalls, tc.Equals, 1)
	c.Check(state.machineKeys["1"], tc.Equals, testPrivateKey)
}

func (s *serviceSuite) TestUnitVirtualHostKeyReturnsExistingAfterConcurrentInsert(c *tc.C) {
	modelUUID := coremodel.UUID(testModelUUID)
	state := newStubModelState()
	state.unitExists["postgresql/0"] = true
	state.unitEnsureKeys["postgresql/0"] = testPrivateKey

	svc := modelsshservice.NewService(modelUUID, state)

	key, err := svc.UnitVirtualHostKey(c.Context(), coreunit.Name("postgresql/0"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(key, tc.Equals, testPrivateKey)
	c.Check(state.unitEnsureCalls, tc.Equals, 1)
	c.Check(state.unitKeys["postgresql/0"], tc.Equals, testPrivateKey)
}

func (s *serviceSuite) TestVirtualHostKeyFromMachineInfo(c *tc.C) {
	modelUUID := coremodel.UUID(testModelUUID)
	state := newStubModelState()
	state.machineExists["1"] = true
	state.machineKeys["1"] = testPrivateKey

	svc := modelsshservice.NewService(modelUUID, state)

	info, err := virtualhostname.NewInfoMachineTarget(testModelUUID, "1")
	c.Assert(err, tc.ErrorIsNil)

	key, err := svc.VirtualHostKey(c.Context(), info)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(key, tc.Equals, testPrivateKey)
}

func (s *serviceSuite) TestVirtualHostKeyErrorsForDifferentModel(c *tc.C) {
	modelUUID := coremodel.UUID(testModelUUID)
	svc := modelsshservice.NewService(modelUUID, newStubModelState())

	info, err := virtualhostname.NewInfoMachineTarget("77f44fa2-65f1-41c8-8a8e-3b1f1c8d343d", "1")
	c.Assert(err, tc.ErrorIsNil)

	_, err = svc.VirtualHostKey(c.Context(), info)
	c.Assert(err, tc.ErrorMatches, `virtual hostname model UUID .* does not match service model .*`)
}

func (s *serviceSuite) TestVirtualHostKeyErrorsForNestedMachine(c *tc.C) {
	modelUUID := coremodel.UUID(testModelUUID)
	svc := modelsshservice.NewService(modelUUID, newStubModelState())

	info, err := virtualhostname.NewInfoMachineTarget(testModelUUID, "1/lxd/0")
	c.Assert(err, tc.ErrorIsNil)

	_, err = svc.VirtualHostKey(c.Context(), info)
	c.Assert(err, tc.ErrorMatches, `cannot SSH directly to nested machine "1/lxd/0", connect to parent machine "1" instead`)
}

type stubModelState struct {
	machineExists      map[string]bool
	machineKeys        map[string]string
	machineAlgos       map[string]int
	unitExists         map[string]bool
	unitKeys           map[string]string
	unitAlgos          map[string]int
	unitMachines       map[string]string
	machineEnsureKeys  map[string]string
	unitEnsureKeys     map[string]string
	machineEnsureCalls int
	unitEnsureCalls    int
}

func newStubModelState() *stubModelState {
	return &stubModelState{
		machineExists:     make(map[string]bool),
		machineKeys:       make(map[string]string),
		machineAlgos:      make(map[string]int),
		unitExists:        make(map[string]bool),
		unitKeys:          make(map[string]string),
		unitAlgos:         make(map[string]int),
		unitMachines:      make(map[string]string),
		machineEnsureKeys: make(map[string]string),
		unitEnsureKeys:    make(map[string]string),
	}
}

func (s *stubModelState) GetMachineVirtualHostKeyByMachineName(_ context.Context, machineName string) (string, bool, error) {
	if !s.machineExists[machineName] {
		return "", false, errors.Errorf("machine %q not found", machineName)
	}
	key, found := s.machineKeys[machineName]
	return key, found, nil
}

func (s *stubModelState) EnsureMachineVirtualHostKeyByMachineName(_ context.Context, machineName string, algorithmTypeID int, key string) (string, error) {
	if !s.machineExists[machineName] {
		return "", errors.Errorf("machine %q not found", machineName)
	}
	if existingKey, ok := s.machineEnsureKeys[machineName]; ok {
		s.machineKeys[machineName] = existingKey
		s.machineEnsureCalls++
		delete(s.machineEnsureKeys, machineName)
		return existingKey, nil
	}
	s.machineKeys[machineName] = key
	s.machineAlgos[machineName] = algorithmTypeID
	s.machineEnsureCalls++
	return key, nil
}

func (s *stubModelState) GetUnitVirtualHostKeyByUnitName(_ context.Context, unitName string) (string, bool, error) {
	if !s.unitExists[unitName] {
		return "", false, errors.Errorf("unit %q not found", unitName)
	}
	key, found := s.unitKeys[unitName]
	return key, found, nil
}

func (s *stubModelState) EnsureUnitVirtualHostKeyByUnitName(_ context.Context, unitName string, algorithmTypeID int, key string) (string, error) {
	if !s.unitExists[unitName] {
		return "", errors.Errorf("unit %q not found", unitName)
	}
	if existingKey, ok := s.unitEnsureKeys[unitName]; ok {
		s.unitKeys[unitName] = existingKey
		s.unitEnsureCalls++
		delete(s.unitEnsureKeys, unitName)
		return existingKey, nil
	}
	s.unitKeys[unitName] = key
	s.unitAlgos[unitName] = algorithmTypeID
	s.unitEnsureCalls++
	return key, nil
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
