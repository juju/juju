// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// This file has routines which can be used for agent specific functionalities related to service files:
//	- finding all agents in the machine
//	- create conf file using the machine details
// 	- write systemd service file and setting links
// 	- copy all tools and related to agents and setup the links
// 	- start all the agents
// These routines can be used by any tools/cmds trying to implement the above functionality as part of the process, eg. juju-updateseries command.

package service

import (
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/fs"
	"github.com/juju/utils/series"
	"github.com/juju/utils/shell"
	"github.com/juju/utils/symlink"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/agent/tools"
	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/service/common"
	"github.com/juju/juju/service/systemd"
)

type SystemdServiceManager interface {
	// FindAgents finds all the agents available in the machine.
	FindAgents(dataDir string) (string, []string, []string, error)

	// WriteSystemdAgents creates systemd files and create symlinks for the list of machine and units passed in the standard filepath.
	WriteSystemdAgents(machineAgent string, unitAgents []string, dataDir string, symLinkSystemdDir string, symLinkSystemdMultiUserDir string, series string) ([]string, []string, []string, error)

	//CreateAgentConf creates the configfile for specified agent running on a host with specified series.
	CreateAgentConf(agentName string, dataDir string, series string) (common.Conf, error)

	// CopyAgentBinary copies all the tools into the path specified for each agent.
	CopyAgentBinary(machineAgent string, unitAgents []string, dataDir string, toSeries string, fromSeries string, jujuVersion version.Number) error

	// StartAllAgents starts all the agents in the machine with specified series.
	StartAllAgents(machineAgent string, unitAgents []string, dataDir string, series string) (string, []string, error)

	// WriteServiceFile writes the service file in '/lib/systemd/system' path.
	// this is done as part of upgrade step.
	WriteServiceFile() error
}

type systemdServiceManager struct {
	isRunning func() (bool, error)
}

// NewSystemdServiceManager returns object of systemServiceManager interface.
func NewSystemdServiceManager(isRunning func() (bool, error)) SystemdServiceManager {
	return &systemdServiceManager{isRunning: isRunning}
}

// FindAgents finds all the agents available in the machine.
func (s *systemdServiceManager) FindAgents(dataDir string) (string, []string, []string, error) {

	var (
		machineAgent  string
		unitAgents    []string
		errAgentNames []string
	)

	agentDir := filepath.Join(dataDir, "agents")
	dir, err := os.Open(agentDir)
	if err != nil {
		return "", nil, nil, errors.Annotate(err, "opening agents dir")
	}
	defer dir.Close()

	entries, err := dir.Readdir(-1)
	if err != nil {
		return "", nil, nil, errors.Annotate(err, "reading agents dir")
	}
	for _, info := range entries {
		name := info.Name()
		tag, err := names.ParseTag(name)
		if err != nil {
			continue
		}
		switch tag.Kind() {
		case names.MachineTagKind:
			machineAgent = name
		case names.UnitTagKind:
			unitAgents = append(unitAgents, name)
		default:
			errAgentNames = append(errAgentNames, name)
			logger.Infof("%s is not of type Machine nor Unit, ignoring", name)
		}
	}
	return machineAgent, unitAgents, errAgentNames, nil
}

// WriteSystemdAgents creates systemd files and create symlinks for the list of machine and units passed in the standard filepath '/var/lib/juju' during the upgrade process.
func (s *systemdServiceManager) WriteSystemdAgents(machineAgent string, unitAgents []string, dataDir string, symLinkSystemdDir string, symLinkSystemdMultiUserDir string, series string) ([]string, []string, []string, error) {

	var (
		startedSysServiceNames []string
		startedSymServiceNames []string
		errAgentNames          []string
		lastError              error
	)

	for _, agentName := range append(unitAgents, machineAgent) {
		conf, err := s.CreateAgentConf(agentName, dataDir, series)
		if err != nil {
			logger.Infof("%s", err)
			lastError = err
			continue
		}

		svcName := serviceName(agentName)
		svc, err := NewService(svcName, conf, series)
		if err != nil {
			logger.Infof("Failed to create new service %s: ", err)
			continue
		}

		upSvc, ok := svc.(UpgradableService)
		if !ok {
			initName, err := VersionInitSystem(series)
			if err != nil {
				return nil, nil, nil, errors.Trace(errors.Annotate(err, "nor is service an UpgradableService"))
			}
			return nil, nil, nil, errors.Errorf("%s service not of type UpgradableService", initName)
		}

		if err = upSvc.WriteService(); err != nil {
			logger.Infof("failed to write service for %s: %s", agentName, err)
			errAgentNames = append(errAgentNames, agentName)
			lastError = err
			continue
		} else {
			logger.Infof("successfully wrote service for %s:", agentName)
		}

		running, err := s.isRunning()
		switch {
		case err != nil:
			return nil, nil, nil, errors.Errorf("failure attempting to determine if systemd is running: %#v\n", err)
		case running:
			// Links for manual and automatic use of the service
			// have been written, move to the next.
			startedSysServiceNames = append(startedSysServiceNames, svcName)
			logger.Infof("wrote %s agent, enabled and linked by systemd", svcName)
			continue
		}

		svcFileName := svcName + ".service"
		if err = os.Symlink(path.Join(dataDir, "init", svcName, svcFileName),
			path.Join(symLinkSystemdDir, svcFileName)); err != nil && !os.IsExist(err) {
			return nil, nil, nil, errors.Errorf("failed to link service file (%s) in systemd dir: %s\n", svcFileName, err)
		}

		if err = os.Symlink(path.Join(dataDir, "init", svcName, svcFileName),
			path.Join(symLinkSystemdMultiUserDir, svcFileName)); err != nil && !os.IsExist(err) {
			return nil, nil, nil, errors.Errorf("failed to link service file (%s) in multi-user.target.wants dir: %s\n", svcFileName, err)
		}

		startedSymServiceNames = append(startedSymServiceNames, svcName)
		logger.Infof("wrote %s agent, enabled and linked by symlink", svcName)
	}
	return startedSysServiceNames, startedSymServiceNames, errAgentNames, lastError
}

// CreateAgentConf creates the configfile for specified agent running on a host with specified series.
func (s *systemdServiceManager) CreateAgentConf(agentName string, dataDir string, series string) (_ common.Conf, err error) {
	defer func() {
		if err != nil {
			logger.Infof("failed create agent conf for %s: %s", agentName, err)
		}
	}()

	renderer, err := shell.NewRenderer("")
	if err != nil {
		return common.Conf{}, err
	}

	tag, err := names.ParseTag(agentName)
	if err != nil {
		return common.Conf{}, err
	}
	name := tag.Id()

	var kind AgentKind
	switch tag.Kind() {
	case names.MachineTagKind:
		kind = AgentKindMachine
	case names.UnitTagKind:
		kind = AgentKindUnit
	default:
		return common.Conf{}, errors.NewNotValid(nil, fmt.Sprintf("agent %q is neither a machine nor a unit", agentName))
	}

	srvPath := path.Join(paths.NixLogDir, "juju")
	info := NewAgentInfo(
		kind,
		name,
		dataDir,
		srvPath)
	return AgentConf(info, renderer), nil
}

// CopyAgentBinary copies all the tools into the path specified for each agent.
func (s *systemdServiceManager) CopyAgentBinary(machineAgent string, unitAgents []string, dataDir string, toSeries string, fromSeries string, jujuVersion version.Number) (err error) {
	defer func() {
		if err != nil {
			err = errors.Annotate(err, "failed to copy tools")
		}
	}()

	// Setup new and old version.Binarys with only the series
	// different.
	fromVers := version.Binary{
		Number: jujuVersion,
		Arch:   arch.HostArch(),
		Series: fromSeries,
	}
	toVers := version.Binary{
		Number: jujuVersion,
		Arch:   arch.HostArch(),
		Series: toSeries,
	}

	// If tools with the new series don't already exist, copy
	// current tools to new directory with correct series.
	if _, err = os.Stat(tools.SharedToolsDir(dataDir, toVers)); err != nil {
		// Copy tools to new directory with correct series.
		if err = fs.Copy(tools.SharedToolsDir(dataDir, fromVers), tools.SharedToolsDir(dataDir, toVers)); err != nil {
			return err
		}
	}

	// Write tools metadata with new version, however don't change
	// the URL, so we know where it came from.
	jujuTools, err := tools.ReadTools(dataDir, toVers)
	if err != nil {
		return errors.Trace(err)
	}

	// Only write once
	if jujuTools.Version != toVers {
		jujuTools.Version = toVers
		if err = tools.WriteToolsMetadataData(tools.ToolsDir(dataDir, toVers.String()), jujuTools); err != nil {
			return err
		}
	}

	// Update Agent Tool links
	var lastError error
	for _, agentName := range append(unitAgents, machineAgent) {
		toolPath := tools.ToolsDir(dataDir, toVers.String())
		toolsDir := tools.ToolsDir(dataDir, agentName)

		err = symlink.Replace(toolsDir, toolPath)
		if err != nil {
			lastError = err
		}
	}

	return lastError
}

// StartAllAgents starts all the agents in the machine with specified series.
func (s *systemdServiceManager) StartAllAgents(machineAgent string, unitAgents []string, dataDir string, series string) (string, []string, error) {

	var (
		startedMachineName string
		startedUnitNames   []string
		err                error
	)

	running, err := s.isRunning()

	switch {
	case err != nil:
		return "", nil, err
	case !running:
		return "", nil, errors.Errorf("systemd is not fully running, please reboot to start agents")
	}

	for _, unit := range unitAgents {
		if err = startAgent(unit, AgentKindUnit, dataDir, series); err != nil {
			return "", nil, errors.Annotatef(err, "failed to start %s service", serviceName(unit))
		}
		startedUnitNames = append(startedUnitNames, serviceName(unit))
		logger.Infof("started %s service", serviceName(unit))
	}

	err = startAgent(machineAgent, AgentKindMachine, dataDir, series)
	if err == nil {
		startedMachineName = serviceName(machineAgent)
		logger.Infof("started %s service", serviceName(machineAgent))
	}
	return startedMachineName, startedUnitNames, errors.Annotatef(err, "failed to start %s service", serviceName(machineAgent))
}

func startAgent(name string, kind AgentKind, dataDir string, series string) (err error) {
	renderer, err := shell.NewRenderer("")
	if err != nil {
		return err
	}
	srvPath := path.Join(paths.NixLogDir, "juju")
	info := NewAgentInfo(
		kind,
		name,
		dataDir,
		srvPath,
	)
	conf := AgentConf(info, renderer)
	svcName := serviceName(name)
	svc, err := NewService(svcName, conf, series)
	if err = svc.Start(); err != nil {
		return err
	}
	return nil
}

func serviceName(agent string) string {
	return "jujud-" + agent
}

// WriteServiceFile writes the service life in standard '/lib/systemd/system' path.
func (s *systemdServiceManager) WriteServiceFile() error {
	hostSeries, err := series.HostSeries()
	if err != nil {
		return err
	}
	dataDir, err := paths.DataDir(hostSeries)
	if err != nil {
		return err
	}

	// find the agents.
	machineAgent, unitAgents, failedAgentNames, err := s.FindAgents(dataDir)
	if err != nil {
		return err
	}

	startedSysdServiceNames, startedSymServiceNames, failedAgentNames, err := s.WriteSystemdAgents(
		machineAgent,
		unitAgents,
		dataDir,
		"/etc/systemd/system",
		"/etc/systemd/system/multi-user.target.wants",
		hostSeries,
	)
	if err != nil {
		for _, agentName := range failedAgentNames {
			logger.Errorf("failed to write service for %s: %s", agentName, err)
		}
		logger.Errorf("%s", err)
		return err
	}
	for _, sysSvcName := range startedSysdServiceNames {
		logger.Infof("wrote %s agent, enabled and linked by systemd", sysSvcName)
	}
	for _, symSvcName := range startedSymServiceNames {
		logger.Infof("wrote %s agent, enabled and linked by symlink", symSvcName)
	}

	// reload the services.
	err = systemd.SysdReload()
	if err != nil {
		return err
	}

	return nil
}
