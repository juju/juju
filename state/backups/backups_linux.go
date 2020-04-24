// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build linux

package backups

import (
	"net"
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/utils/shell"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/service"
	"github.com/juju/juju/state"
	"github.com/juju/juju/version"
)

func ensureMongoService(agentConfig agent.Config) error {
	var oplogSize int
	if oplogSizeString := agentConfig.Value(agent.MongoOplogSize); oplogSizeString != "" {
		var err error
		if oplogSize, err = strconv.Atoi(oplogSizeString); err != nil {
			return errors.Annotatef(err, "invalid oplog size: %q", oplogSizeString)
		}
	}

	var numaCtlPolicy bool
	if numaCtlString := agentConfig.Value(agent.NUMACtlPreference); numaCtlString != "" {
		var err error
		if numaCtlPolicy, err = strconv.ParseBool(numaCtlString); err != nil {
			return errors.Annotatef(err, "invalid numactl preference: %q", numaCtlString)
		}
	}

	si, ok := agentConfig.StateServingInfo()
	if !ok {
		return errors.Errorf("agent config has no state serving info")
	}

	if err := mongo.EnsureServiceInstalled(agentConfig.DataDir(),
		si.StatePort,
		oplogSize,
		numaCtlPolicy,
		agentConfig.MongoVersion(),
		true,
		mongo.MemoryProfileDefault,
	); err != nil {
		return errors.Annotate(err, "cannot ensure that mongo service start/stop scripts are in place")
	}
	// Installing a service will not automatically restart it.
	if err := mongo.StartService(); err != nil {
		return errors.Annotate(err, "failed to start mongo")
	}
	return nil
}

// Restore handles either returning or creating a controller to a backed up status:
// * extracts the content of the given backup file and:
// * runs mongorestore with the backed up mongo dump
// * updates and writes configuration files
// * updates existing db entries to make sure they hold no references to
// old instances
// * updates config in all agents.
func (b *backups) Restore(backupId string, args RestoreArgs) (names.Tag, error) {
	meta, backupReader, err := b.Get(backupId)
	if err != nil {
		return nil, errors.Annotatef(err, "could not fetch backup %q", backupId)
	}

	defer backupReader.Close()

	workspace, err := NewArchiveWorkspaceReader(backupReader)
	if err != nil {
		return nil, errors.Annotate(err, "cannot unpack backup file")
	}
	defer workspace.Close()

	// This might actually work, but we don't have a guarantee so we don't allow it.
	if meta.Origin.Series != args.NewInstSeries {
		return nil, errors.Errorf("cannot restore a backup made in a machine with series %q into a machine with series %q, %#v", meta.Origin.Series, args.NewInstSeries, meta)
	}

	// TODO(perrito666) Create a compatibility table of sorts.
	vers := meta.Origin.Version
	if vers.Major != 2 {
		return nil, errors.Errorf("Juju version %v cannot restore backups made using Juju version %v", version.Current.Minor, vers)
	}
	backupMachine := names.NewMachineTag(meta.Origin.Machine)

	// The path for the config file might change if the tag changed
	// and also the rest of the path, so we assume as little as possible.
	oldDatadir, err := paths.DataDir(args.NewInstSeries)
	if err != nil {
		return nil, errors.Annotate(err, "cannot determine DataDir for the restored machine")
	}

	var oldAgentConfig agent.ConfigSetterWriter
	oldAgentConfigFile := agent.ConfigPath(oldDatadir, args.NewInstTag)
	if oldAgentConfig, err = agent.ReadConfig(oldAgentConfigFile); err != nil {
		return nil, errors.Annotate(err, "cannot load old agent config from disk")
	}

	logger.Infof("stopping juju-db")
	if err = mongo.StopService(); err != nil {
		return nil, errors.Annotate(err, "failed to stop mongo")
	}

	// delete all the files to be replaced
	if err := PrepareMachineForRestore(oldAgentConfig.MongoVersion()); err != nil {
		return nil, errors.Annotate(err, "cannot delete existing files")
	}
	logger.Infof("deleted old files to place new")

	if err := workspace.UnpackFilesBundle(filesystemRoot()); err != nil {
		return nil, errors.Annotate(err, "cannot obtain system files from backup")
	}
	logger.Infof("placed new restore files")

	var agentConfig agent.ConfigSetterWriter
	// The path for the config file might change if the tag changed
	// and also the rest of the path, so we assume as little as possible.
	datadir, err := paths.DataDir(args.NewInstSeries)
	if err != nil {
		return nil, errors.Annotate(err, "cannot determine DataDir for the restored machine")
	}
	agentConfigFile := agent.ConfigPath(datadir, backupMachine)
	if agentConfig, err = agent.ReadConfig(agentConfigFile); err != nil {
		return nil, errors.Annotate(err, "cannot load agent config from disk")
	}
	ssi, ok := agentConfig.StateServingInfo()
	if !ok {
		return nil, errors.Errorf("cannot determine state serving info")
	}

	// TODO (manadart 2019-08-23): This needs fixing.
	// APIHostPorts are created without space IDs, which could mean a loss of
	// information and broken functionality between backup and restore.

	APIHostPorts := network.NewSpaceHostPorts(ssi.APIPort, args.PrivateAddress, args.PublicAddress)
	agentConfig.SetAPIHostPorts([]network.HostPorts{APIHostPorts.HostPorts()})
	if err := agentConfig.Write(); err != nil {
		return nil, errors.Annotate(err, "cannot write new agent configuration")
	}

	logger.Infof("wrote new agent config for restore")

	if backupMachine.Id() != "0" {
		logger.Infof("extra work needed backup belongs to %q machine", backupMachine.String())
		serviceName := "jujud-" + agentConfig.Tag().String()
		aInfo := service.NewMachineAgentInfo(
			agentConfig.Tag().Id(),
			dataDir,
			paths.MustSucceed(paths.LogDir(args.NewInstSeries)),
		)

		// TODO(perrito666) renderer should have a RendererForSeries, for the moment
		// restore only works on linuxes.
		renderer, _ := shell.NewRenderer("bash")
		serviceAgentConf := service.AgentConf(aInfo, renderer)
		svc, err := service.NewService(serviceName, serviceAgentConf, args.NewInstSeries)
		if err != nil {
			return nil, errors.Annotate(err, "cannot generate service for the restored agent.")
		}
		if err := svc.Install(); err != nil {
			return nil, errors.Annotate(err, "cannot install service for the restored agent.")
		}
		logger.Infof("new machine service")
	}

	logger.Infof("mongo service will be reinstalled to ensure its presence")
	if err := ensureMongoService(agentConfig); err != nil {
		return nil, errors.Annotate(err, "failed to reinstall service for juju-db")
	}

	dialInfo, err := newDialInfo(args.PrivateAddress, agentConfig)
	if err != nil {
		return nil, errors.Annotate(err, "cannot produce dial information")
	}

	// For the unresponsive controller case the oldAgentConfig and agentConfig
	// have different certificates. MongoDB has been already started with a
	// new certificate. Therefore all clients that would like to communicate
	// with mongo should use the new certificate otherwise the
	// "TLS handshake error" occurs. To avoid this error the old certificate
	// should be replaced by the new one.
	oldAgentConfig.SetCACert(agentConfig.CACert())
	oldDialInfo, err := newDialInfo(args.PrivateAddress, oldAgentConfig)
	if err != nil {
		return nil, errors.Annotate(err, "cannot produce dial information for existing mongo")
	}

	logger.Infof("new mongo will be restored")
	mgoVer := agentConfig.MongoVersion()

	tagUser, tagUserPassword, err := tagUserCredentials(agentConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}
	rArgs := RestorerArgs{
		DialInfo:        dialInfo,
		Version:         mgoVer,
		TagUser:         tagUser,
		TagUserPassword: tagUserPassword,
		RunCommandFn:    runCommand,
		StartMongo:      mongo.StartService,
		StopMongo:       mongo.StopService,
		NewMongoSession: NewMongoSession,
		GetDB:           GetDB,
	}

	// Restore mongodb from backup
	restorer, err := NewDBRestorer(rArgs)
	if err != nil {
		return nil, errors.Annotate(err, "error preparing for restore")
	}
	if err := restorer.Restore(workspace.DBDumpDir, oldDialInfo); err != nil {
		return nil, errors.Annotate(err, "error restoring state from backup")
	}

	// Re-start replicaset with the new value for server address
	logger.Infof("restarting replicaset")
	memberHostPort := net.JoinHostPort(args.PrivateAddress, strconv.Itoa(ssi.StatePort))
	err = resetReplicaSet(dialInfo, memberHostPort)
	if err != nil {
		return nil, errors.Annotate(err, "cannot reset replicaSet")
	}

	err = updateMongoEntries(args.NewInstId, args.NewInstTag.Id(), backupMachine.Id(), dialInfo)
	if err != nil {
		return nil, errors.Annotate(err, "cannot update mongo entries")
	}

	// From here we work with the restored controller
	mgoInfo, ok := agentConfig.MongoInfo()
	if !ok {
		return nil, errors.Errorf("cannot retrieve info to connect to mongo")
	}

	pool, err := connectToDB(agentConfig.Controller(), agentConfig.Model(), mgoInfo)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer pool.Close()
	st := pool.SystemState()

	machine, err := st.Machine(backupMachine.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}

	logger.Infof("updating local machine addresses")
	err = updateMachineAddresses(machine, args.PrivateAddress, args.PublicAddress)
	if err != nil {
		return nil, errors.Annotate(err, "cannot update api server machine addresses")
	}
	// Update the APIHostPorts as well. Under normal circumstances the API
	// Host Ports are only set during bootstrap and by the peergrouper worker.
	// Unfortunately right now, the peer grouper is busy restarting and isn't
	// guaranteed to set the host ports before the remote machines we are
	// about to tell about us. If it doesn't, the remote machine gets its
	// agent.conf file updated with this new machine's IP address, it then
	// starts, and the "api-address-updater" worker asks for the api host
	// ports, and gets told the old IP address of the machine that was backed
	// up. It then writes this incorrect file to its agent.conf file, which
	// causes it to attempt to reconnect to the api server. Unfortunately it
	// now has the wrong address and can never get the  correct one.
	// So, we set it explicitly here.
	if err := st.SetAPIHostPorts([]network.SpaceHostPorts{APIHostPorts}); err != nil {
		return nil, errors.Annotate(err, "cannot update api server host ports")
	}

	// update all agents known to the new controller.
	// TODO(perrito666): We should never stop process because of this.
	// updateAllMachines will not return errors for individual
	// agent update failures

	modelUUIDs, err := st.AllModelUUIDs()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var machines []machineModel
	for _, modelUUID := range modelUUIDs {
		st, err := pool.Get(modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		defer func() {
			st.Release()
		}()

		model, err := st.Model()
		if err != nil {
			return nil, errors.Trace(err)
		}

		machinesForModel, err := st.AllMachines()
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, machine := range machinesForModel {
			machines = append(machines, machineModel{machine: machine, model: model})
		}
	}
	logger.Infof("updating other machine addresses")
	if err := updateAllMachines(args.PrivateAddress, args.PublicAddress, machines); err != nil {
		return nil, errors.Annotate(err, "cannot update agents")
	}

	// Mark restoreInfo as Finished so upon restart of the apiserver
	// the client can reconnect and determine if we where successful.
	info := st.RestoreInfo()
	// In mongo 3.2, even though the backup is made with --oplog, there
	// are stale transactions in this collection.
	if err := info.PurgeTxn(); err != nil {
		return nil, errors.Annotate(err, "cannot purge stale transactions")
	}
	if err = info.SetStatus(state.RestoreFinished); err != nil {
		return nil, errors.Annotate(err, "failed to set status to finished")
	}

	return backupMachine, nil
}
