// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"context"
	stdtesting "testing"
	"time"

	"github.com/canonical/gomock/gomock"
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
	sshstate "github.com/juju/juju/domain/ssh/state/model"
)

type serviceSuite struct{}

func TestServiceSuite(t *stdtesting.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) TestMachineVirtualHostKeyGeneratesMissing(c *tc.C) {
	modelUUID := coremodel.UUID(testModelUUID)
	state := NewMockState(gomock.NewController(c))
	state.EXPECT().GetMachineVirtualHostKeyByMachineName(gomock.Any(), "1").Return("", false, nil)
	state.EXPECT().EnsureMachineVirtualHostKeyByMachineName(gomock.Any(), "1", domainssh.SSHKeyAlgorithmTypeED25519ID, gomock.Any()).Return(testPrivateKey, nil)

	svc := modelsshservice.NewService(state, modelUUID, clock.WallClock)

	key, err := svc.MachineVirtualHostKey(c.Context(), coremachine.Name("1"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(key, tc.Equals, testPrivateKey)
	assertPrivateKey(c, key)
}

func (s *serviceSuite) TestUnitVirtualHostKeyUsesBackingMachine(c *tc.C) {
	modelUUID := coremodel.UUID(testModelUUID)
	state := NewMockState(gomock.NewController(c))
	state.EXPECT().GetMachineNameForUnit(gomock.Any(), "postgresql/0").Return("1", true, nil)
	state.EXPECT().GetMachineVirtualHostKeyByMachineName(gomock.Any(), "1").Return(testPrivateKey, true, nil)

	svc := modelsshservice.NewService(state, modelUUID, clock.WallClock)

	key, err := svc.UnitVirtualHostKey(c.Context(), coreunit.Name("postgresql/0"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(key, tc.Equals, testPrivateKey)
}

func (s *serviceSuite) TestUnitVirtualHostKeyGeneratesMissingForCAAS(c *tc.C) {
	modelUUID := coremodel.UUID(testModelUUID)
	state := NewMockState(gomock.NewController(c))
	state.EXPECT().GetMachineNameForUnit(gomock.Any(), "postgresql/0").Return("", false, nil)
	state.EXPECT().GetUnitVirtualHostKeyByUnitName(gomock.Any(), "postgresql/0").Return("", false, nil)
	state.EXPECT().EnsureUnitVirtualHostKeyByUnitName(gomock.Any(), "postgresql/0", domainssh.SSHKeyAlgorithmTypeED25519ID, gomock.Any()).Return(testPrivateKey, nil)

	svc := modelsshservice.NewService(state, modelUUID, clock.WallClock)

	key, err := svc.UnitVirtualHostKey(c.Context(), coreunit.Name("postgresql/0"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(key, tc.Equals, testPrivateKey)
	assertPrivateKey(c, key)
}

func (s *serviceSuite) TestMachineVirtualHostKeyEnsureReturnsKey(c *tc.C) {
	modelUUID := coremodel.UUID(testModelUUID)
	state := NewMockState(gomock.NewController(c))
	state.EXPECT().GetMachineVirtualHostKeyByMachineName(gomock.Any(), "1").Return("", false, nil)
	state.EXPECT().EnsureMachineVirtualHostKeyByMachineName(gomock.Any(), "1", domainssh.SSHKeyAlgorithmTypeED25519ID, gomock.Any()).Return(testPrivateKey, nil)

	svc := modelsshservice.NewService(state, modelUUID, clock.WallClock)

	key, err := svc.MachineVirtualHostKey(c.Context(), coremachine.Name("1"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(key, tc.Equals, testPrivateKey)
}

func (s *serviceSuite) TestUnitVirtualHostKeyEnsureReturnsKey(c *tc.C) {
	modelUUID := coremodel.UUID(testModelUUID)
	state := NewMockState(gomock.NewController(c))
	state.EXPECT().GetMachineNameForUnit(gomock.Any(), "postgresql/0").Return("", false, nil)
	state.EXPECT().GetUnitVirtualHostKeyByUnitName(gomock.Any(), "postgresql/0").Return("", false, nil)
	state.EXPECT().EnsureUnitVirtualHostKeyByUnitName(gomock.Any(), "postgresql/0", domainssh.SSHKeyAlgorithmTypeED25519ID, gomock.Any()).Return(testPrivateKey, nil)

	svc := modelsshservice.NewService(state, modelUUID, clock.WallClock)

	key, err := svc.UnitVirtualHostKey(c.Context(), coreunit.Name("postgresql/0"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(key, tc.Equals, testPrivateKey)
}

func (s *serviceSuite) TestVirtualHostKeyFromMachineInfo(c *tc.C) {
	modelUUID := coremodel.UUID(testModelUUID)
	state := NewMockState(gomock.NewController(c))
	state.EXPECT().GetMachineVirtualHostKeyByMachineName(gomock.Any(), "1").Return(testPrivateKey, true, nil)

	svc := modelsshservice.NewService(state, modelUUID, clock.WallClock)

	info, err := virtualhostname.NewInfoMachineTarget(testModelUUID, "1")
	c.Assert(err, tc.ErrorIsNil)

	key, err := svc.VirtualHostKey(c.Context(), info)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(key, tc.Equals, testPrivateKey)
}

func (s *serviceSuite) TestVirtualHostKeyErrorsForDifferentModel(c *tc.C) {
	modelUUID := coremodel.UUID(testModelUUID)
	svc := modelsshservice.NewService(NewMockState(gomock.NewController(c)), modelUUID, clock.WallClock)

	info, err := virtualhostname.NewInfoMachineTarget("77f44fa2-65f1-41c8-8a8e-3b1f1c8d343d", "1")
	c.Assert(err, tc.ErrorIsNil)

	_, err = svc.VirtualHostKey(c.Context(), info)
	c.Assert(err, tc.ErrorMatches, `virtual hostname model UUID .* does not match service model .*`)
}

func (s *serviceSuite) TestVirtualHostKeyErrorsForNestedMachine(c *tc.C) {
	modelUUID := coremodel.UUID(testModelUUID)
	svc := modelsshservice.NewService(NewMockState(gomock.NewController(c)), modelUUID, clock.WallClock)

	info, err := virtualhostname.NewInfoMachineTarget(testModelUUID, "1/lxd/0")
	c.Assert(err, tc.ErrorIsNil)

	_, err = svc.VirtualHostKey(c.Context(), info)
	c.Assert(err, tc.ErrorMatches, `cannot SSH directly to nested machine "1/lxd/0", connect to parent machine "1" instead`)
}

func (s *serviceSuite) TestResolveK8sExecInfo(c *tc.C) {
	state := NewMockState(gomock.NewController(c))
	state.EXPECT().GetModelInfo(gomock.Any()).Return(sshstate.ModelInfo{Type: string(coremodel.CAAS), Name: "test-model"}, nil)
	state.EXPECT().GetUnitK8sPodInfo(gomock.Any(), "app/0").Return("pod-id", nil)
	svc := modelsshservice.NewService(state, coremodel.UUID(testModelUUID), clock.WallClock)
	info, err := virtualhostname.NewInfoUnitTarget(testModelUUID, "app/0")
	c.Assert(err, tc.ErrorIsNil)

	namespace, podName, err := svc.ResolveK8sExecInfo(c.Context(), info)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(namespace, tc.Equals, "test-model")
	c.Check(podName, tc.Equals, "pod-id")
}

func (s *serviceSuite) TestMachineForDestination(c *tc.C) {
	state := NewMockState(gomock.NewController(c))
	state.EXPECT().GetModelInfo(gomock.Any()).Return(sshstate.ModelInfo{Type: string(coremodel.IAAS)}, nil)
	state.EXPECT().CheckMachineExists(gomock.Any(), "1").Return(true, nil)
	svc := modelsshservice.NewService(state, coremodel.UUID(testModelUUID), clock.WallClock)
	info, err := virtualhostname.NewInfoMachineTarget(testModelUUID, "1")
	c.Assert(err, tc.ErrorIsNil)

	machineName, err := svc.MachineForDestination(c.Context(), info)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(machineName, tc.Equals, coremachine.Name("1"))
}

func (s *serviceSuite) TestInsertSSHConnRequest(c *tc.C) {
	clk := testclock.NewClock(time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	modelUUID := coremodel.UUID(testModelUUID)
	req := domainssh.SSHConnRequest{
		TunnelID:            testTunnelUUID,
		MachineName:         "1",
		Expires:             clk.Now().Add(time.Minute),
		ControllerAddresses: network.NewSpaceAddresses("10.0.0.1", "10.0.0.2"),
		UnitPort:            22,
		EphemeralPublicKey:  []byte("key"),
	}
	state := NewMockState(gomock.NewController(c))
	state.EXPECT().InsertSSHConnRequest(gomock.Any(), req, clk.Now()).Return(nil)
	svc := modelsshservice.NewService(state, modelUUID, clk)

	err := svc.InsertSSHConnRequest(c.Context(), req)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestInsertSSHConnRequestRejectsExpired(c *tc.C) {
	clk := testclock.NewClock(time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	modelUUID := coremodel.UUID(testModelUUID)
	state := NewMockState(gomock.NewController(c))
	svc := modelsshservice.NewService(state, modelUUID, clk)

	req := domainssh.SSHConnRequest{
		TunnelID:    testTunnelUUID,
		MachineName: "1",
		Expires:     clk.Now().Add(-time.Minute),
	}

	err := svc.InsertSSHConnRequest(c.Context(), req)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *serviceSuite) TestGetSSHConnRequest(c *tc.C) {
	clk := testclock.NewClock(time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	modelUUID := coremodel.UUID(testModelUUID)
	state := NewMockState(gomock.NewController(c))
	getReq := domainssh.SSHConnRequest{TunnelID: testTunnelUUID, MachineName: "1"}
	state.EXPECT().GetSSHConnRequest(gomock.Any(), "1", testTunnelUUID, clk.Now()).Return(getReq, nil)
	svc := modelsshservice.NewService(state, modelUUID, clk)

	req, err := svc.GetSSHConnRequest(c.Context(), coremachine.Name("1"), testTunnelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(req, tc.DeepEquals, getReq)
}

// TestWatchSSHConnRequest checks that the watcher is scoped to the requesting
// machine: the machine UUID is resolved, the prune runs, and the watcher is
// created against the ssh_connection_request namespace.
func (s *serviceSuite) TestWatchSSHConnRequest(c *tc.C) {
	clk := testclock.NewClock(time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	modelUUID := coremodel.UUID(testModelUUID)
	state := NewMockState(gomock.NewController(c))
	state.EXPECT().PruneExpiredSSHConnRequests(gomock.Any(), clk.Now()).Return(nil)
	state.EXPECT().GetMachineUUIDByName(gomock.Any(), "0").Return("machine-uuid-0", nil)
	state.EXPECT().InitialWatchSSHConnRequestsStatement().Return("ssh_connection_request", "SELECT tunnel_id FROM ssh_connection_request WHERE machine_uuid = ?")
	watcherFactory := &stubWatcherFactory{watcher: watchertest.NewMockStringsWatcher(make(chan []string))}
	svc := modelsshservice.NewWatchableService(state, modelUUID, clk, watcherFactory)

	w, err := svc.WatchSSHConnRequest(c.Context(), coremachine.Name("0"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(w, tc.Equals, watcherFactory.watcher)
	c.Check(watcherFactory.summary, tc.Equals, "ssh connection request watcher")
	c.Check(watcherFactory.namespace, tc.Equals, "ssh_connection_request")
}

func (s *serviceSuite) TestRemoveSSHConnRequest(c *tc.C) {
	modelUUID := coremodel.UUID(testModelUUID)
	state := NewMockState(gomock.NewController(c))
	state.EXPECT().RemoveSSHConnRequest(gomock.Any(), testTunnelUUID).Return(nil)
	svc := modelsshservice.NewService(state, modelUUID, clock.WallClock)

	err := svc.RemoveSSHConnRequest(c.Context(), testTunnelUUID)
	c.Assert(err, tc.ErrorIsNil)
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
