// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"net"
	"os"
	"path/filepath"
	"strconv"

	"github.com/hashicorp/raft"
	"github.com/juju/errors"
	"github.com/juju/replicaset"

	raftworker "github.com/juju/juju/worker/raft"
)

// jujuMachineKey is the key for the replset member tag where we
// store the member's corresponding machine id.
const jujuMachineKey = "juju-machine-id"

// BootstrapRaft initialises the raft cluster in a controller that is
// being upgraded.
func BootstrapRaft(context Context) error {
	agentConfig := context.AgentConfig()
	storageDir := filepath.Join(agentConfig.DataDir(), "raft")
	_, err := os.Stat(storageDir)
	// If the storage dir already exists we shouldn't run again. (If
	// we statted the dir successfully, this will return nil.)
	if !os.IsNotExist(err) {
		return err
	}
	_, transport := raft.NewInmemTransport(raft.ServerAddress("notused"))
	defer transport.Close()

	conf, err := raftworker.NewRaftConfig(raftworker.Config{
		LocalID:   raft.ServerID(agentConfig.Tag().Id()),
		Logger:    logger,
		Transport: transport,
		FSM:       raftworker.BootstrapFSM{},
	})
	if err != nil {
		return errors.Annotate(err, "getting raft config")
	}
	logStore, err := raftworker.NewLogStore(storageDir)
	if err != nil {
		return errors.Annotate(err, "making log store")
	}
	defer logStore.Close()

	snapshotStore, err := raftworker.NewSnapshotStore(storageDir, 2, logger)
	if err != nil {
		return errors.Annotate(err, "making snapshot store")
	}

	st := context.State()
	members, err := st.ReplicaSetMembers()
	if err != nil {
		return errors.Annotate(err, "getting replica set members")
	}
	info, err := st.StateServingInfo()
	if err != nil {
		return errors.Annotate(err, "getting state serving info")
	}
	servers, err := makeRaftServers(members, info.APIPort)
	if err != nil {
		return errors.Trace(err)
	}
	err = raft.BootstrapCluster(conf, logStore, logStore, snapshotStore, transport, servers)
	return errors.Annotate(err, "bootstrapping raft cluster")
}

func makeRaftServers(members []replicaset.Member, apiPort int) (raft.Configuration, error) {
	var empty raft.Configuration
	var servers []raft.Server
	for _, member := range members {
		id, ok := member.Tags[jujuMachineKey]
		if !ok {
			return empty, errors.NotFoundf("juju machine id for replset member %d", member.Id)
		}
		baseAddress, _, err := net.SplitHostPort(member.Address)
		if err != nil {
			return empty, errors.Annotatef(err, "getting base address for replset member %d", member.Id)
		}
		apiAddress := net.JoinHostPort(baseAddress, strconv.Itoa(apiPort))
		suffrage := raft.Voter
		if member.Votes != nil && *member.Votes < 1 {
			suffrage = raft.Nonvoter
		}
		server := raft.Server{
			ID:       raft.ServerID(id),
			Address:  raft.ServerAddress(apiAddress),
			Suffrage: suffrage,
		}
		servers = append(servers, server)
	}
	return raft.Configuration{Servers: servers}, nil
}
