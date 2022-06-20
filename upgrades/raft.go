// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"bytes"
	"net"
	"path/filepath"
	"strconv"

	"github.com/hashicorp/go-msgpack/codec"
	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
	"github.com/juju/errors"
	"github.com/juju/replicaset/v2"

	"github.com/juju/juju/agent"
	raftworker "github.com/juju/juju/worker/raft"
)

// jujuMachineKey is the key for the replset member tag where we
// store the member's corresponding machine id.
const jujuMachineKey = "juju-machine-id"

var errLogEmpty = errors.Errorf("no log entries, expected at least one for configuration")

// BootstrapRaft initialises the raft cluster in a controller that is
// being upgraded.
func BootstrapRaft(context Context) error {
	agentConfig := context.AgentConfig()
	storageDir := raftDir(agentConfig)

	// Always sync raft log writes when upgrading.
	logStore, err := raftworker.NewLogStore(storageDir, raftworker.SyncAfterWrite)
	if err != nil {
		return errors.Annotate(err, "making log store")
	}
	defer logStore.Close()

	snapshotStore, err := raftworker.NewSnapshotStore(storageDir, 2, logger)
	if err != nil {
		return errors.Annotate(err, "making snapshot store")
	}

	// If there is already a configuration entry in the log store then
	// it's already been bootstrapped.
	_, err = getCombinedState(logStore, snapshotStore)
	if err == errLogEmpty {
		// This is what we want - no configuration means that bootstrapping is required.
	} else if err == nil {
		// This is already bootstrapped, we can just stop.
		return nil
	} else if err != nil {
		return errors.Annotate(err, "checking for existing configuration log entry")
	}

	return errors.Trace(bootstrapWithStores(context, logStore, snapshotStore))
}

func bootstrapWithStores(
	context Context,
	logStore *raftboltdb.BoltStore,
	snapshotStore raft.SnapshotStore,
) error {
	_, transport := raft.NewInmemTransport(raft.ServerAddress("notused"))
	defer transport.Close()

	agentConfig := context.AgentConfig()
	conf, err := raftworker.NewRaftConfig(raftworker.Config{
		LocalID:   raft.ServerID(agentConfig.Tag().Id()),
		Logger:    logger,
		Transport: transport,
		FSM:       raftworker.BootstrapFSM{},
	})
	if err != nil {
		return errors.Annotate(err, "getting raft config")
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

func raftDir(agentConfig agent.ConfigSetter) string {
	return filepath.Join(agentConfig.DataDir(), "raft")
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

type combinedStoreState struct {
	lastIndex   uint64
	lastTerm    uint64
	configIndex uint64
	config      raft.Configuration
}

// getCombinedState gets the last index, term and configuration (with
// index) from the logs and snapshot store passed in.
func getCombinedState(logs raft.LogStore, snapshots raft.SnapshotStore) (*combinedStoreState, error) {
	lastLog, lastConfig, err := collectLogEntries(logs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	snapshot, err := getLastSnapshot(snapshots)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var result combinedStoreState
	if snapshot == nil && lastLog == nil {
		return nil, errLogEmpty
	}

	if snapshot == nil && lastConfig == nil {
		// This shouldn't be possible - a configuration is needed to
		// commit entries to the log.
		return nil, errors.Errorf("log entries but no configuration found")
	}

	if snapshot != nil {
		result.lastIndex = snapshot.Index
		result.lastTerm = snapshot.Term
		result.configIndex = snapshot.ConfigurationIndex
		result.config = snapshot.Configuration
	}

	if lastLog != nil && lastLog.Index >= result.lastIndex {
		result.lastIndex = lastLog.Index
		result.lastTerm = lastLog.Term
	}

	if lastConfig != nil && lastConfig.Index >= result.configIndex {
		result.configIndex = lastConfig.Index
		result.config, err = decodeConfiguration(lastConfig.Data)
		if err != nil {
			return nil, errors.Annotate(err, "decoding last configuration")
		}
	}
	return &result, nil
}

// collectLogEntries returns two log entries: the latest one, and the
// most recent configuration entry. (These might be the same.)
func collectLogEntries(store raft.LogStore) (*raft.Log, *raft.Log, error) {
	var latest raft.Log

	lastIndex, err := store.LastIndex()
	if err != nil {
		return nil, nil, errors.Annotate(err, "getting last index")
	}

	if lastIndex == 0 {
		return nil, nil, nil
	}

	err = store.GetLog(lastIndex, &latest)
	if err != nil {
		return nil, nil, errors.Annotate(err, "getting last log entry")
	}

	if latest.Type == raft.LogConfiguration {
		return &latest, &latest, nil
	}

	firstIndex, err := store.FirstIndex()
	if err != nil {
		return nil, nil, errors.Annotate(err, "getting first index`")
	}
	current := lastIndex
	for current > firstIndex {
		current--
		var entry raft.Log
		err := store.GetLog(current, &entry)
		if errors.Cause(err) == raft.ErrLogNotFound {
			continue
		} else if err != nil {
			return nil, nil, errors.Annotatef(err, "getting log index %d", current)
		}
		if entry.Type == raft.LogConfiguration {
			return &latest, &entry, nil
		}
	}

	return &latest, nil, nil
}

// loadLastSnapshot returns the metadata of the last snapshot in the
// store, if any.
func getLastSnapshot(store raft.SnapshotStore) (*raft.SnapshotMeta, error) {
	snapshots, err := store.List()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, snapshot := range snapshots {
		_, source, err := store.Open(snapshot.ID)
		if err != nil {
			logger.Warningf("couldn't open snapshot %q: %v", snapshot.ID, err)
			continue
		}
		source.Close()
		return snapshot, nil
	}
	if len(snapshots) > 0 {
		return nil, errors.Errorf("couldn't open any existing snapshots")
	}
	return nil, nil
}

func decodeConfiguration(data []byte) (raft.Configuration, error) {
	var hd codec.MsgpackHandle
	dec := codec.NewDecoder(bytes.NewBuffer(data), &hd)
	var config raft.Configuration
	err := dec.Decode(&config)
	return config, errors.Trace(err)
}
