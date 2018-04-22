// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/juju/utils/series"
	"github.com/juju/utils/shell"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/service/common"
	"github.com/juju/juju/service/systemd"
)

// Service has setting used to the process of installing the service files
type systemdService struct {
	machineAgent	string
	unitAgents	[]string
	dataDir		string
	systemdDir	string
	newDBus		systemd.DBusAPIFactory
}

//WriteServieFile
func (s *systemdService) WriteServiceFile() error {

	hostSeries, err := series.HostSeries()
	if err != nil {
		return errors.Trace(err)
	}
	/* Find the agents */
	err = s.findSystemdAgents(hostSeries)
	if nil != err {
		return err
	}

	err = s.writeSystemdAgents(hostSeries)
	if nil != err {
		return err
	}

	return nil
}

func (s *systemdService) findSystemdAgents(series string) error {

	dataDir := paths.MustSucceed(paths.DataDir(series))
	agentsDir := filepath.Join(dataDir, "agents")
	dir, err := os.Open(agentsDir)
	if err != nil {
		return errors.Annotate(err, "opening agents dir")
	}
	defer dir.Close()

	entries, err := dir.Readdir(-1)
	if err != nil {
		return errors.Annotate(err, "reading agents dir")
	}
	for _, info := range entries {
		name := info.Name()
		tag, err := names.ParseTag(name)
		if err != nil {
			continue
		}
		switch tag.Kind() {
			case names.MachineTagKind:
				s.machineAgent = name
			case names.UnitTagKind:
				s.unitAgents = append(s.unitAgents, name)
			default:
				logger.Infof("%s is not of type Machine nor Unit, ignoring", name)
		}
	}
	return nil
}

func (s *systemdService) writeSystemdAgents(series string) error {
	systemdDir := paths.MustSucceed(paths.SystemdDir(series))
	symLinkSystemdDir := "/etc/systemd/system"
	symLinkSystemdMultiUserDir := symLinkSystemdDir + "/multi-user.target.wants"
	var lastError error

	for _, agentName := range append(s.unitAgents, s.machineAgent) {
		conf, err := s.createAgentConf(agentName, series)
		if err != nil {
			logger.Infof("%s", err)
			lastError = err
			continue
		}
		svcName := "jujud-" + agentName
		svc, err := systemd.NewService(svcName, conf, systemdDir, s.newDBus)
		if err = svc.WriteService(); err != nil {
			logger.Infof("failed to write service for %s: %s", agentName, err)
			lastError = err
			continue
		}

		running, err := systemd.IsRunning()
		switch {
			case nil != err:
				return errors.Errorf("failure attempting to determine if systemd is running: %#v\n", err)
			case running:
				//Links are written
				logger.Infof("wrote %s agent, enabled and linked by systemd", svcName)
				continue
		}

		svcFileName := svcName + ".service"
		if err = os.Symlink(path.Join(s.systemdDir, "juju-init", svcName, svcFileName),
			path.Join(symLinkSystemdDir, svcFileName)); err != nil && !os.IsExist(err) {
				return errors.Errorf("failed to link service file (%s) in systemd dir: %s\n", svcFileName, err)
		}

		if err = os.Symlink(path.Join(s.systemdDir, "juju-init", svcName, svcFileName),
			path.Join(symLinkSystemdMultiUserDir, svcFileName)); err != nil && !os.IsExist(err) {
				return errors.Errorf("failed to link service file (%s) in multi-user.target.wants dir: %s\n", svcFileName, err)
		}
		logger.Infof("wrote %s agent, enabled and linked by symlink", svcName)
	}
	return lastError
}


func (s *systemdService) createAgentConf(agentName string, series string) (_ common.Conf, err error) {
	defer func() {
		if nil != err {
			logger.Infof("Failed create agent conf for %s: %s", agentName, err)
		}
	}()

	renderer, err := shell.NewRenderer("")
	if nil != err {
		return common.Conf{}, err
	}

	tag, err := names.ParseTag(agentName)
	if nil != err {
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

	info := NewAgentInfo(
		kind,
		name,
		s.dataDir,
		paths.MustSucceed(paths.LogDir(series)),
	)
	return AgentConf(info, renderer), nil
}
