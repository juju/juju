// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"context"
	stdtesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/tc"
	gossh "golang.org/x/crypto/ssh"

	coreerrors "github.com/juju/juju/core/errors"
	coremachine "github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/virtualhostname"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/core/watcher/watchertest"
	domainssh "github.com/juju/juju/domain/ssh"
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

	svc := modelsshservice.NewService(state, modelUUID, clock.WallClock)

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

	svc := modelsshservice.NewService(state, modelUUID, clock.WallClock)

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

	svc := modelsshservice.NewService(state, modelUUID, clock.WallClock)

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

	svc := modelsshservice.NewService(state, modelUUID, clock.WallClock)

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

	svc := modelsshservice.NewService(state, modelUUID, clock.WallClock)

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

	svc := modelsshservice.NewService(state, modelUUID, clock.WallClock)

	info, err := virtualhostname.NewInfoMachineTarget(testModelUUID, "1")
	c.Assert(err, tc.ErrorIsNil)

	key, err := svc.VirtualHostKey(c.Context(), info)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(key, tc.Equals, testPrivateKey)
}

func (s *serviceSuite) TestVirtualHostKeyErrorsForDifferentModel(c *tc.C) {
	modelUUID := coremodel.UUID(testModelUUID)
	svc := modelsshservice.NewService(newStubModelState(), modelUUID, clock.WallClock)

	info, err := virtualhostname.NewInfoMachineTarget("77f44fa2-65f1-41c8-8a8e-3b1f1c8d343d", "1")
	c.Assert(err, tc.ErrorIsNil)

	_, err = svc.VirtualHostKey(c.Context(), info)
	c.Assert(err, tc.ErrorMatches, `virtual hostname model UUID .* does not match service model .*`)
}

func (s *serviceSuite) TestVirtualHostKeyErrorsForNestedMachine(c *tc.C) {
	modelUUID := coremodel.UUID(testModelUUID)
	svc := modelsshservice.NewService(newStubModelState(), modelUUID, clock.WallClock)

	info, err := virtualhostname.NewInfoMachineTarget(testModelUUID, "1/lxd/0")
	c.Assert(err, tc.ErrorIsNil)

	_, err = svc.VirtualHostKey(c.Context(), info)
	c.Assert(err, tc.ErrorMatches, `cannot SSH directly to nested machine "1/lxd/0", connect to parent machine "1" instead`)
}

func (s *serviceSuite) TestInsertSSHConnRequest(c *tc.C) {
	clk := testclock.NewClock(time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	modelUUID := coremodel.UUID(testModelUUID)
	state := newStubModelState()
	state.machineExists["1"] = true
	svc := modelsshservice.NewService(state, modelUUID, clk)

	req := domainssh.SSHConnRequest{
		TunnelID:            testTunnelUUID,
		MachineName:         "1",
		Expires:             clk.Now().Add(time.Minute),
		SSHUsername:         "juju-reverse-tunnel",
		SSHPassword:         "secret",
		ControllerAddresses: network.NewSpaceAddresses("10.0.0.1", "10.0.0.2"),
		UnitPort:            22,
		EphemeralPublicKey:  []byte("key"),
	}

	err := svc.InsertSSHConnRequest(c.Context(), req)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(state.insertedReq, tc.DeepEquals, req)
	c.Check(state.insertNow, tc.Equals, clk.Now())
}

func (s *serviceSuite) TestInsertSSHConnRequestRejectsExpired(c *tc.C) {
	clk := testclock.NewClock(time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	modelUUID := coremodel.UUID(testModelUUID)
	state := newStubModelState()
	state.machineExists["1"] = true
	svc := modelsshservice.NewService(state, modelUUID, clk)

	req := domainssh.SSHConnRequest{
		TunnelID:    testTunnelUUID,
		MachineName: "1",
		Expires:     clk.Now().Add(-time.Minute),
		SSHUsername: "juju-reverse-tunnel",
		SSHPassword: "secret",
	}

	err := svc.InsertSSHConnRequest(c.Context(), req)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *serviceSuite) TestGetSSHConnRequest(c *tc.C) {
	clk := testclock.NewClock(time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	modelUUID := coremodel.UUID(testModelUUID)
	state := newStubModelState()
	state.getReq = domainssh.SSHConnRequest{TunnelID: testTunnelUUID, MachineName: "1"}
	svc := modelsshservice.NewService(state, modelUUID, clk)

	req, err := svc.GetSSHConnRequest(c.Context(), testTunnelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(req, tc.DeepEquals, state.getReq)
	c.Check(state.getTunnelID, tc.Equals, testTunnelUUID)
	c.Check(state.getNow, tc.Equals, clk.Now())
}

// TestWatchSSHConnRequest checks that the watcher is scoped to the requesting
// machine: the machine UUID is resolved, the prune runs, and the watcher is
// created against the ssh_connection_request namespace.
func (s *serviceSuite) TestWatchSSHConnRequest(c *tc.C) {
	clk := testclock.NewClock(time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	modelUUID := coremodel.UUID(testModelUUID)
	state := newStubModelState()
	state.machineUUIDs["0"] = "machine-uuid-0"
	watcherFactory := &stubWatcherFactory{watcher: watchertest.NewMockStringsWatcher(make(chan []string))}
	svc := modelsshservice.NewWatchableService(state, modelUUID, clk, watcherFactory)

	w, err := svc.WatchSSHConnRequest(c.Context(), coremachine.Name("0"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(w, tc.Equals, watcherFactory.watcher)
	c.Check(state.pruneNow, tc.Equals, clk.Now())
	c.Check(state.machineUUIDName, tc.Equals, coremachine.Name("0"))
	c.Check(watcherFactory.summary, tc.Equals, "ssh connection request watcher")
	c.Check(watcherFactory.namespace, tc.Equals, "ssh_connection_request")
}

func (s *serviceSuite) TestRemoveSSHConnRequest(c *tc.C) {
	modelUUID := coremodel.UUID(testModelUUID)
	state := newStubModelState()
	svc := modelsshservice.NewService(state, modelUUID, clock.WallClock)

	err := svc.RemoveSSHConnRequest(c.Context(), testTunnelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(state.removedTunnelID, tc.Equals, testTunnelUUID)
}

type stubModelState struct {
	machineEnsureKeys  map[string]string
	unitEnsureKeys     map[string]string
	machineEnsureCalls int
	unitEnsureCalls    int
	machineExists      map[string]bool
	machineKeys        map[string]string
	machineAlgos       map[string]int
	unitExists         map[string]bool
	unitKeys           map[string]string
	unitAlgos          map[string]int
	unitMachines       map[string]string
	insertedReq        domainssh.SSHConnRequest
	insertNow          time.Time
	getReq             domainssh.SSHConnRequest
	getTunnelID        string
	getNow             time.Time
	pruneNow           time.Time
	removedTunnelID    string
	machineUUIDs       map[string]string
	machineUUIDName    coremachine.Name
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
		machineUUIDs:      make(map[string]string),
	}
}

func (s *stubModelState) GetMachineVirtualHostKeyByMachineName(_ context.Context, machineName string) (string, bool, error) {
	if !s.machineExists[machineName] {
		return "", false, errors.Errorf("machine %q not found", machineName)
	}
	key, found := s.machineKeys[machineName]
	return key, found, nil
}

// EnsureMachineVirtualHostKeyByMachineName(context.Context, string, int, string) (string, error)
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

func (s *stubModelState) InsertSSHConnRequest(_ context.Context, req domainssh.SSHConnRequest, now time.Time) error {
	s.insertedReq = req
	s.insertNow = now
	return nil
}

func (s *stubModelState) GetSSHConnRequest(_ context.Context, tunnelID string, now time.Time) (domainssh.SSHConnRequest, error) {
	s.getTunnelID = tunnelID
	s.getNow = now
	return s.getReq, nil
}

func (s *stubModelState) RemoveSSHConnRequest(_ context.Context, tunnelID string) error {
	s.removedTunnelID = tunnelID
	return nil
}

func (s *stubModelState) PruneExpiredSSHConnRequests(_ context.Context, now time.Time) error {
	s.pruneNow = now
	return nil
}

func (s *stubModelState) GetMachineUUIDByName(_ context.Context, machineName coremachine.Name) (string, error) {
	s.machineUUIDName = machineName
	uuid, ok := s.machineUUIDs[machineName.String()]
	if !ok {
		return "", errors.Errorf("machine %q not found", machineName)
	}
	return uuid, nil
}

func (*stubModelState) FilterSSHConnRequestsForMachine(_ context.Context, tunnelIDs []string, _ string) ([]string, error) {
	return tunnelIDs, nil
}

func (*stubModelState) InitialWatchSSHConnRequestsStatement() (string, string) {
	return "ssh_connection_request", "SELECT tunnel_id FROM ssh_connection_request WHERE machine_uuid = ?"
}

type stubWatcherFactory struct {
	watcher   watcher.StringsWatcher
	namespace string
	summary   string
}

func (s *stubWatcherFactory) NewNamespaceMapperWatcher(
	_ context.Context,
	_ eventsource.NamespaceQuery,
	summary string,
	_ eventsource.Mapper,
	filterOption eventsource.FilterOption,
	_ ...eventsource.FilterOption,
) (watcher.StringsWatcher, error) {
	s.summary = summary
	s.namespace = filterOption.Namespace()
	return s.watcher, nil
}

func assertPrivateKey(c *tc.C, key string) {
	_, err := gossh.ParsePrivateKey([]byte(key))
	c.Assert(err, tc.ErrorIsNil)
}

const (
	testModelUUID  = "8419cd78-4993-4c3a-928e-c646226beeee"
	testTunnelUUID = "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
	testPrivateKey = "-----BEGIN OPENSSH PRIVATE KEY-----\n" +
		"b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtz\n" +
		"c2gtZWQyNTUxOQAAACBT8UidoqUmpUFFCGEhZhHWGE7VHoJY7LZ7yXzuWlSVYAAA\n" +
		"AIiZq0wRmatMEQAAAAtzc2gtZWQyNTUxOQAAACBT8UidoqUmpUFFCGEhZhHWGE7V\n" +
		"HoJY7LZ7yXzuWlSVYAAAAEBYRsJTytYJUidtOuv3s3tdjyDA+4TSdCz9+hFKjyqz\n" +
		"v1PxSJ2ipSalQUUIYSFmEdYYTtUegljstnvJfO5aVJVgAAAAAAECAwQF\n" +
		"-----END OPENSSH PRIVATE KEY-----\n"
)
