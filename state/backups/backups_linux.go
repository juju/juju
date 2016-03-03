// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build linux

package backups

import (
	"net"
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils/shell"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/service"
	"github.com/juju/juju/state"
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
	if numaCtlString := agentConfig.Value(agent.NumaCtlPreference); numaCtlString != "" {
		var err error
		if numaCtlPolicy, err = strconv.ParseBool(numaCtlString); err != nil {
			return errors.Annotatef(err, "invalid numactl preference: %q", numaCtlString)
		}
	}

	si, ok := agentConfig.StateServingInfo()
	if !ok {
		return errors.Errorf("agent config has no state serving info")
	}

	// TODO(perrito666) determine mongo version from dump.
	err := mongo.EnsureServiceInstalled(agentConfig.DataDir(),
		si.StatePort,
		oplogSize,
		numaCtlPolicy,
		mongo.Mongo24,
		true,
	)
	return errors.Annotate(err, "cannot ensure that mongo service start/stop scripts are in place")
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

	// TODO(perrito666) Create a compatibility table of sorts.
	version := meta.Origin.Version
	backupMachine := names.NewMachineTag(meta.Origin.Machine)

	if err := mongo.StopService(); err != nil {
		return nil, errors.Annotate(err, "cannot stop mongo to replace files")
	}

	// delete all the files to be replaced
	if err := PrepareMachineForRestore(); err != nil {
		return nil, errors.Annotate(err, "cannot delete existing files")
	}
	logger.Infof("deleted old files to place new")

	if err := workspace.UnpackFilesBundle(filesystemRoot()); err != nil {
		return nil, errors.Annotate(err, "cannot obtain system files from backup")
	}
	logger.Infof("placed new files")

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
	APIHostPorts := network.NewHostPorts(ssi.APIPort, args.PrivateAddress)
	agentConfig.SetAPIHostPorts([][]network.HostPort{APIHostPorts})
	if err := agentConfig.Write(); err != nil {
		return nil, errors.Annotate(err, "cannot write new agent configuration")
	}
	logger.Infof("wrote new agent config")

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

	logger.Infof("new mongo will be restored")
	// Restore mongodb from backup
	if err := placeNewMongoService(workspace.DBDumpDir, version); err != nil {
		return nil, errors.Annotate(err, "error restoring state from backup")
	}

	// Re-start replicaset with the new value for server address
	dialInfo, err := newDialInfo(args.PrivateAddress, agentConfig)
	if err != nil {
		return nil, errors.Annotate(err, "cannot produce dial information")
	}

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

	st, err := newStateConnection(agentConfig.Model(), mgoInfo)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer st.Close()

	machine, err := st.Machine(backupMachine.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}

	err = updateMachineAddresses(machine, args.PrivateAddress, args.PublicAddress)
	if err != nil {
		return nil, errors.Annotate(err, "cannot update api server machine addresses")
	}

	// update all agents known to the new controller.
	// TODO(perrito666): We should never stop process because of this.
	// updateAllMachines will not return errors for individual
	// agent update failures
	machines, err := st.AllMachines()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err = updateAllMachines(args.PrivateAddress, machines); err != nil {
		return nil, errors.Annotate(err, "cannot update agents")
	}

	info, err := st.RestoreInfoSetter()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Mark restoreInfo as Finished so upon restart of the apiserver
	// the client can reconnect and determine if we where succesful.
	err = info.SetStatus(state.RestoreFinished)

	return backupMachine, errors.Annotate(err, "failed to set status to finished")
}
