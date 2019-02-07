// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"bytes"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/hashicorp/go-msgpack/codec"
	"github.com/hashicorp/raft"
	"github.com/juju/errors"
	"github.com/juju/replicaset"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/raftlease"
	"github.com/juju/juju/feature"
	raftworker "github.com/juju/juju/worker/raft"
)

// jujuMachineKey is the key for the replset member tag where we
// store the member's corresponding machine id.
const jujuMachineKey = "juju-machine-id"

// BootstrapRaft initialises the raft cluster in a controller that is
// being upgraded.
func BootstrapRaft(context Context) error {
	agentConfig := context.AgentConfig()
	storageDir := raftDir(agentConfig)
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
	// * and there are no snapshots in the snapshot store (which shows
	//   that the raft-lease store is already in use).
	st := context.State()
	controllerConfig, err := st.ControllerConfig()
	if err != nil {
		return errors.Annotate(err, "getting controller config")
	}
	if controllerConfig.Features().Contains(feature.LegacyLeases) {
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
	snapshotStore, err := raftworker.NewSnapshotStore(
		storageDir, 2, logger)
	if err != nil {
		return errors.Annotate(err, "opening snapshot store")
	}
	snapshots, err := snapshotStore.List()
	if err != nil {
		return errors.Annotate(err, "listing snapshots")
	}
	if len(snapshots) != 0 {
		logger.Debugf("snapshots found in store - raft leases in use")
		return nil
	}

	// We need the last term and index, latest configuration and
	// configuration index from the log store.
	logStore, err := raftworker.NewLogStore(storageDir)
	if err != nil {
		return errors.Annotate(err, "opening log store")
	}
	defer logStore.Close()

	latest, configEntry, err := collectLogEntries(logStore)
	if err != nil {
		return errors.Trace(err)
	}

	configuration, err := decodeConfiguration(configEntry.Data)
	if err != nil {
		return errors.Annotate(err, "decoding configuration")
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
		latest.Index,
		latest.Term,
		configuration,
		configEntry.Index,
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

// collectLogEntries returns two log entries: the latest one, and the
// most recent configuration entry. (These might be the same.)
func collectLogEntries(store raft.LogStore) (*raft.Log, *raft.Log, error) {
	var latest raft.Log

	lastIndex, err := store.LastIndex()
	if err != nil {
		return nil, nil, errors.Annotate(err, "getting last index")
	}

	if lastIndex == 0 {
		return nil, nil, errors.Errorf("no log entries, expected at least one for configuration")
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

	return nil, nil, errors.Errorf("no configuration entry in log")
}

func decodeConfiguration(data []byte) (raft.Configuration, error) {
	var hd codec.MsgpackHandle
	dec := codec.NewDecoder(bytes.NewBuffer(data), &hd)
	var config raft.Configuration
	err := dec.Decode(&config)
	return config, errors.Trace(err)
}
