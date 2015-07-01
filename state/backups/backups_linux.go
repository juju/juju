// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build linux

package backups

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

// Restore handles either returning or creating a state server to a backed up status:
// * extracts the content of the given backup file and:
// * runs mongorestore with the backed up mongo dump
// * updates and writes configuration files
// * updates existing db entries to make sure they hold no references to
// old instances
// * updates config in all agents.
func (b *backups) Restore(backupId string, args RestoreArgs) error {
	meta, backupReader, err := b.Get(backupId)
	if err != nil {
		return errors.Annotatef(err, "could not fetch backup %q", backupId)
	}

	defer backupReader.Close()

	workspace, err := NewArchiveWorkspaceReader(backupReader)
	if err != nil {
		return errors.Annotate(err, "cannot unpack backup file")
	}
	defer workspace.Close()

	// TODO(perrito666) Create a compatibility table of sorts.
	version := meta.Origin.Version
	backupMachine := names.NewMachineTag(meta.Origin.Machine)

	// delete all the files to be replaced
	if err := PrepareMachineForRestore(); err != nil {
		return errors.Annotate(err, "cannot delete existing files")
	}

	if err := workspace.UnpackFilesBundle(filesystemRoot()); err != nil {
		return errors.Annotate(err, "cannot obtain system files from backup")
	}

	if err := updateBackupMachineTag(backupMachine, args.NewInstTag); err != nil {
		return errors.Annotate(err, "cannot update paths to reflect current machine id")
	}

	var agentConfig agent.ConfigSetterWriter
	// The path for the config file might change if the tag changed
	// and also the rest of the path, so we assume as little as possible.
	datadir, err := paths.DataDir(args.NewInstSeries)
	if err != nil {
		return errors.Annotate(err, "cannot determine DataDir for the restored machine")
	}
	agentConfigFile := agent.ConfigPath(datadir, args.NewInstTag)
	if agentConfig, err = agent.ReadConfig(agentConfigFile); err != nil {
		return errors.Annotate(err, "cannot load agent config from disk")
	}
	ssi, ok := agentConfig.StateServingInfo()
	if !ok {
		return errors.Errorf("cannot determine state serving info")
	}
	// The machine tag might have changed, we update it.
	agentConfig.SetValue("tag", args.NewInstTag.String())
	apiHostPorts := [][]network.HostPort{
		network.NewHostPorts(ssi.APIPort, args.PrivateAddress),
	}
	agentConfig.SetAPIHostPorts(apiHostPorts)
	if err := agentConfig.Write(); err != nil {
		return errors.Annotate(err, "cannot write new agent configuration")
	}

	// Restore mongodb from backup
	if err := placeNewMongo(workspace.DBDumpDir, version); err != nil {
		return errors.Annotate(err, "error restoring state from backup")
	}

	// Re-start replicaset with the new value for server address
	dialInfo, err := newDialInfo(args.PrivateAddress, agentConfig)
	if err != nil {
		return errors.Annotate(err, "cannot produce dial information")
	}

	memberHostPort := fmt.Sprintf("%s:%d", args.PrivateAddress, ssi.StatePort)
	err = resetReplicaSet(dialInfo, memberHostPort)
	if err != nil {
		return errors.Annotate(err, "cannot reset replicaSet")
	}

	err = updateMongoEntries(args.NewInstId, args.NewInstTag.Id(), backupMachine.Id(), dialInfo)
	if err != nil {
		return errors.Annotate(err, "cannot update mongo entries")
	}

	// From here we work with the restored state server
	mgoInfo, ok := agentConfig.MongoInfo()
	if !ok {
		return errors.Errorf("cannot retrieve info to connect to mongo")
	}

	st, err := newStateConnection(agentConfig.Environment(), mgoInfo)
	if err != nil {
		return errors.Trace(err)
	}
	defer st.Close()

	machine, err := st.Machine(args.NewInstTag.Id())
	if err != nil {
		return errors.Trace(err)
	}

	err = updateMachineAddresses(machine, args.PrivateAddress, args.PublicAddress)
	if err != nil {
		return errors.Annotate(err, "cannot update api server machine addresses")
	}

	// update all agents known to the new state server.
	// TODO(perrito666): We should never stop process because of this.
	// updateAllMachines will not return errors for individual
	// agent update failures
	machines, err := st.AllMachines()
	if err != nil {
		return errors.Trace(err)
	}
	if err = updateAllMachines(args.PrivateAddress, machines); err != nil {
		return errors.Annotate(err, "cannot update agents")
	}

	info, err := st.RestoreInfoSetter()

	if err != nil {
		return errors.Trace(err)
	}

	// Mark restoreInfo as Finished so upon restart of the apiserver
	// the client can reconnect and determine if we where succesful.
	err = info.SetStatus(state.RestoreFinished)

	return errors.Annotate(err, "failed to set status to finished")
}
