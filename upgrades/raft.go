// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"bytes"
	"io/ioutil"
	"net"
	"path/filepath"
	"strconv"
	"time"

	"github.com/hashicorp/go-msgpack/codec"
	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
	"github.com/juju/errors"
	"github.com/juju/replicaset"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/raftlease"
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

	logStore, err := raftworker.NewLogStore(storageDir)
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

// MigrateLegacyLeases converts leases in the legacy store into
// corresponding ones in the raft store.
func MigrateLegacyLeases(context Context) error {
	// We know at this point in time that the raft workers aren't
	// running - they're all guarded by the upgrade-steps gate.

	// We need to migrate leases if:
	// * legacy-leases is off,
	// * there are some legacy leases,
	// * there aren't already leases in the store.
	st := context.State()
	// TODO(legacy-leases): remove this.
	if false {
		logger.Debugf("legacy-leases flag is set, not migrating leases")
		return nil
	}

	var zero time.Time
	legacyLeases, err := st.LegacyLeases(zero)
	if err != nil {
		return errors.Annotate(err, "getting legacy leases")
	}
	if len(legacyLeases) == 0 {
		logger.Debugf("no legacy leases to migrate")
		return nil
	}

	storageDir := raftDir(context.AgentConfig())

	logStore, err := raftworker.NewLogStore(storageDir)
	if err != nil {
		return errors.Annotate(err, "opening log store")
	}
	defer logStore.Close()

	snapshotStore, err := raftworker.NewSnapshotStore(
		storageDir, 2, logger)
	if err != nil {
		return errors.Annotate(err, "opening snapshot store")
	}

	hasLeases, err := leasesInStore(logStore, snapshotStore)
	if err != nil {
		return errors.Trace(err)
	}
	if hasLeases {
		logger.Debugf("snapshots found in store - raft leases in use")
		return nil
	}

	// We need the last term and index, latest configuration and
	// configuration index from the log store.
	storeState, err := getCombinedState(logStore, snapshotStore)
	if err == errLogEmpty {
		// This cluster hasn't been bootstrapped.
		logger.Infof("raft cluster is uninitialised - bootstrapping before migrating leases")
		err = bootstrapWithStores(context, logStore, snapshotStore)
		if err != nil {
			return errors.Annotate(err, "bootstrapping new raft cluster")
		}
		// Re-get the state from the cluster - if this fails it'll be
		// caught by the error check below.
		storeState, err = getCombinedState(logStore, snapshotStore)
	}
	if err != nil {
		return errors.Trace(err)
	}

	entries := make(map[raftlease.SnapshotKey]raftlease.SnapshotEntry, len(legacyLeases))
	target := st.LeaseNotifyTarget(ioutil.Discard, logger)

	// Populate the snapshot and the leaseholders collection.
	for key, info := range legacyLeases {
		if key.Lease == "" || info.Holder == "" {
			logger.Debugf("not migrating blank lease %#v holder %q", key, info.Holder)
			continue
		}
		entries[raftlease.SnapshotKey{
			Namespace: key.Namespace,
			ModelUUID: key.ModelUUID,
			Lease:     key.Lease,
		}] = raftlease.SnapshotEntry{
			Holder:   info.Holder,
			Start:    zero,
			Duration: info.Expiry.Sub(zero),
		}
		target.Claimed(key, info.Holder)
	}

	newSnapshot := raftlease.Snapshot{
		Version:    raftlease.SnapshotVersion,
		Entries:    entries,
		GlobalTime: zero,
	}
	// Store the snapshot.
	_, transport := raft.NewInmemTransport(raft.ServerAddress("notused"))
	defer transport.Close()
	sink, err := snapshotStore.Create(
		raft.SnapshotVersionMax,
		storeState.lastIndex,
		storeState.lastTerm,
		storeState.config,
		storeState.configIndex,
		transport,
	)
	if err != nil {
		return errors.Annotate(err, "creating snapshot sink")
	}
	defer sink.Close()
	err = newSnapshot.Persist(sink)
	if err != nil {
		sink.Cancel()
		return errors.Annotate(err, "persisting snapshot")
	}

	return nil
}

// leasesInStore returns whether the logs and snapshots contain any
// lease information (in which case we shouldn't migrate again).
func leasesInStore(logStore raft.LogStore, snapshotStore raft.SnapshotStore) (bool, error) {
	// There are leases in the store if either the last snapshot (if
	// any) can be loaded by a raftlease FSM, or there are command
	// entries in the log.
	snapshots, err := snapshotStore.List()
	if err != nil {
		return false, errors.Annotate(err, "listing snapshots")
	}
	if len(snapshots) > 0 {
		snapshot := snapshots[0]
		_, source, err := snapshotStore.Open(snapshot.ID)
		if err != nil {
			return false, errors.Annotatef(err, "opening snapshot %q", snapshot.ID)
		}
		defer source.Close()
		fsm := raftlease.NewFSM()
		if fsm.Restore(source) == nil {
			// The fact that the snapshot could be loaded into the FSM
			// means that there are leases stored.
			return true, nil
		}
	}

	// Otherwise we need to check for command entries in the log.
	first, err := logStore.FirstIndex()
	if err != nil {
		return false, errors.Annotate(err, "getting first index from log store")
	}
	last, err := logStore.LastIndex()
	if err != nil {
		return false, errors.Annotate(err, "getting last index from log store")
	}
	for i := first; i <= last; i++ {
		var entry raft.Log
		err := logStore.GetLog(i, &entry)
		if err == raft.ErrLogNotFound {
			continue
		}
		if err != nil {
			return false, errors.Annotatef(err, "getting log %d", i)
		}
		if entry.Type == raft.LogCommand {
			return true, nil
		}
	}
	return false, nil
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
