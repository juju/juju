// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// This file has routines which can be used for agent specific functionality
// related to service files:
//	- finding all agents in the machine
//	- create conf file using the machine details
// 	- write systemd service file and setting links
// 	- copy all tools and related to agents and setup the links
// 	- start all the agents
// These routines can be used by any tools/cmds trying to implement the above
// functionality as part of the process, eg. upgrade-series.

// TODO (manadart 2018-07-31) This module is specific to systemd and should
// reside in the service/systemd package.

package service

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/fs"
	"github.com/juju/utils/shell"
	"github.com/juju/utils/symlink"
	"github.com/juju/version"

	"github.com/juju/juju/agent/tools"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/service/common"
	"github.com/juju/juju/service/systemd"
)

type SystemdServiceManager interface {
	// FindAgents finds all the agents available in the machine.
	FindAgents(dataDir string) (string, []string, []string, error)

	// WriteSystemdAgents creates systemd files and create symlinks for the
	// machine passed in the standard filepath.
	WriteSystemdAgents(
		machineAgent string, dataDir, symLinkSystemdMultiUserDir string,
	) error

	//CreateAgentConf creates the configfile for specified agent running on a
	// host with specified series.
	CreateAgentConf(agentName string, dataDir string) (common.Conf, error)

	// CopyAgentBinary copies all the tools into the path specified for each agent.
	CopyAgentBinary(
		machineAgent string, unitAgents []string, dataDir, toSeries, fromSeries string, jujuVersion version.Number,
	) error

	// StartAllAgents starts all the agents in the machine with specified series.
	StartAllAgents(machineAgent string, dataDir string) (string, error)

	// WriteServiceFiles writes the service files for machine and unit agents
	// in the /etc/systemd/system path.
	WriteServiceFiles() error
}

type systemdServiceManager struct {
	isRunning  func() bool
	newService func(string, common.Conf) (Service, error)
}

// NewServiceManagerWithDefaults returns a SystemdServiceManager created with
// sensible defaults.
func NewServiceManagerWithDefaults() SystemdServiceManager {
	return NewServiceManager(
		systemd.IsRunning,
		func(name string, conf common.Conf) (Service, error) {
			return systemd.NewServiceWithDefaults(name, conf)
		},
	)
}

// NewServiceManager allows creation of a new SystemdServiceManager from
// custom dependencies.
func NewServiceManager(
	isRunning func() bool,
	newService func(string, common.Conf) (Service, error),
) SystemdServiceManager {
	return &systemdServiceManager{
		isRunning:  isRunning,
		newService: newService,
	}
}

// WriteServiceFiles writes service files to the standard
// /etc/systemd/system path.
func (s *systemdServiceManager) WriteServiceFiles() error {
	machineAgent, _, _, err := s.FindAgents(paths.NixDataDir)
	if err != nil {
		return errors.Trace(err)
	}

	err = s.WriteSystemdAgents(
		machineAgent,
		paths.NixDataDir,
		systemd.EtcSystemdMultiUserDir,
	)
	if err != nil {
		return errors.Trace(err)
	}

	return errors.Trace(systemd.SysdReload())
}

// FindAgents finds all the agents available on the machine.
func (s *systemdServiceManager) FindAgents(dataDir string) (string, []string, []string, error) {
	return FindAgents(dataDir)
}

// WriteSystemdAgents creates systemd files and symlinks for the input machine
// and unit agents, in the standard filepath '/var/lib/juju'.
func (s *systemdServiceManager) WriteSystemdAgents(
	machineAgent string, dataDir, systemdMultiUserDir string,
) error {
	systemdLinked, err := s.writeSystemdAgent(machineAgent, dataDir, systemdMultiUserDir)
	if err != nil {
		return err
	}

	if systemdLinked {
		logger.Infof("wrote %s agent, enabled and linked by systemd", serviceName(machineAgent))
	} else {
		logger.Infof("wrote %s agent, enabled and linked by symlink", serviceName(machineAgent))
	}
	return nil
}

// writeSystemdAgent creates systemd files and symlinks for the input agentName.
// The boolean return indicates whether systemd automatically linked the file
// into the multi-user-target directory.
func (s *systemdServiceManager) writeSystemdAgent(agentName, dataDir, systemdMultiUserDir string) (bool, error) {
	conf, err := s.CreateAgentConf(agentName, dataDir)
	if err != nil {
		return false, errors.Trace(err)
	}

	svcName := serviceName(agentName)
	svc, err := s.newService(svcName, conf)
	if err != nil {
		return false, errors.Annotate(err, "creating new service")
	}

	uSvc, ok := svc.(UpgradableService)
	if !ok {
		return false, errors.New("service not of type UpgradableService")
	}

	if err = uSvc.RemoveOldService(); err != nil {
		return false, errors.Annotate(err, "deleting legacy service directory")
	}

	dbusMethodFound := true
	if err = uSvc.WriteService(); err != nil {
		// Note that this error is already logged by the systemd package.

		// This is not ideal, but it is possible on an Upstart-based OS
		// (such as Trusty) for run/systemd/system to exist, which is used
		// for detection of systemd as the running init system.
		// If this happens, then D-Bus will error with the message below.
		// We need to detect this condition and fall through to linking the
		// service files manually.
		if !strings.Contains(strings.ToLower(err.Error()), "no such method") {
			return false, errors.Trace(err)
		} else {
			dbusMethodFound = false
			logger.Infof("attempting to manually link service file for %s", agentName)
		}
	} else {
		logger.Infof("successfully wrote service for %s:", agentName)
	}

	// If systemd is the running init system on this host, *and* if the
	// call to DBusAPI.LinkUnitFiles in WriteService above returned no
	// error, it will have resulted in updated sym-links for the file.
	// We are done.
	if s.isRunning() && dbusMethodFound {
		logger.Infof("wrote %s agent, enabled and linked by systemd", svcName)
		return true, nil
	}

	// Otherwise we need to manually ensure the service unit links.
	svcFileName := svcName + ".service"
	if err = os.Symlink(path.Join(systemd.EtcSystemdDir, svcFileName),
		path.Join(systemdMultiUserDir, svcFileName)); err != nil && !os.IsExist(err) {
		return false, errors.Annotatef(err, "linking service file (%s) in multi-user.target.wants dir", svcFileName)
	}

	logger.Infof("wrote %s agent, enabled and linked by symlink", svcName)
	return false, nil
}

// CreateAgentConf creates the configfile for specified agent running on a host with specified series.
func (s *systemdServiceManager) CreateAgentConf(name string, dataDir string) (_ common.Conf, err error) {
	defer func() {
		if err != nil {
			logger.Infof("failed create agent conf for %s: %s", name, err)
		}
	}()

	renderer, err := shell.NewRenderer("")
	if err != nil {
		return common.Conf{}, err
	}

	tag, err := names.ParseTag(name)
	if err != nil {
		return common.Conf{}, err
	}

	var kind AgentKind
	switch tag.Kind() {
	case names.MachineTagKind:
		kind = AgentKindMachine
	case names.UnitTagKind:
		kind = AgentKindUnit
	default:
		return common.Conf{}, errors.NewNotValid(nil, fmt.Sprintf("agent %q is neither a machine nor a unit", name))
	}

	srvPath := path.Join(paths.NixLogDir, "juju")
	info := NewAgentInfo(kind, tag.Id(), dataDir, srvPath)
	return AgentConf(info, renderer), nil
}

// CopyAgentBinary copies all the tools into the path specified for each agent.
func (s *systemdServiceManager) CopyAgentBinary(
	machineAgent string, unitAgents []string, dataDir, toSeries, fromSeries string, jujuVersion version.Number,
) (err error) {
	defer func() {
		if err != nil {
			err = errors.Annotate(err, "copying agent binaries")
		}
	}()

	// Setup new and old version.Binary instances with different series.
	fromVer := version.Binary{
		Number: jujuVersion,
		Arch:   arch.HostArch(),
		Series: fromSeries,
	}
	toVer := version.Binary{
		Number: jujuVersion,
		Arch:   arch.HostArch(),
		Series: toSeries,
	}

	// If tools with the new series don't already exist, copy
	// current tools to new directory with correct series.
	if _, err = os.Stat(tools.SharedToolsDir(dataDir, toVer)); err != nil {
		// Copy tools to new directory with correct series.
		if err = fs.Copy(tools.SharedToolsDir(dataDir, fromVer), tools.SharedToolsDir(dataDir, toVer)); err != nil {
			return errors.Trace(err)
		}
	}

	// Write tools metadata with new version, however don't change
	// the URL, so we know where it came from.
	jujuTools, err := tools.ReadTools(dataDir, toVer)
	if err != nil {
		return errors.Trace(err)
	}

	// Only write once
	if jujuTools.Version != toVer {
		jujuTools.Version = toVer
		if err = tools.WriteToolsMetadataData(tools.ToolsDir(dataDir, toVer.String()), jujuTools); err != nil {
			return errors.Trace(err)
		}
	}

	// Update Agent Tool links
	for _, agentName := range append(unitAgents, machineAgent) {
		toolPath := tools.ToolsDir(dataDir, toVer.String())
		toolsDir := tools.ToolsDir(dataDir, agentName)

		if err = symlink.Replace(toolsDir, toolPath); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

// StartAllAgents starts all of the input agents.
func (s *systemdServiceManager) StartAllAgents(machineAgent string, dataDir string) (string, error) {
	if !s.isRunning() {
		return "", errors.Errorf("cannot interact with systemd; reboot to start agents")
	}

	machineService := serviceName(machineAgent)
	err := s.startAgent(machineAgent, AgentKindMachine, dataDir)
	if err == nil {
		logger.Infof("started %s service", machineService)
		return machineService, nil
	}

	return "", errors.Annotatef(err, "failed to start %s service", machineService)
}

func (s *systemdServiceManager) startAgent(name string, kind AgentKind, dataDir string) (err error) {
	renderer, err := shell.NewRenderer("")
	if err != nil {
		return errors.Trace(err)
	}

	srvPath := path.Join(paths.NixLogDir, "juju")
	info := NewAgentInfo(kind, name, dataDir, srvPath)
	conf := AgentConf(info, renderer)

	svc, err := s.newService(serviceName(name), conf)
	if err != nil {
		return errors.Trace(err)
	}

	return errors.Trace(svc.Start())
}

func serviceName(agent string) string {
	return "jujud-" + agent
}
