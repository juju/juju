// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upstart

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"

	"github.com/juju/errors"
	"github.com/juju/utils"

	"github.com/juju/juju/service/common"
)

// TODO(ericsnow) Eliminate MachineAgentUpstartService (use NewAgentService).

const maxAgentFiles = 20000

// MachineAgentUpstartService returns the upstart config for a machine agent
// based on the tag and machineId passed in.
func MachineAgentUpstartService(name, toolsDir, dataDir, logDir, tag, machineId string, env map[string]string) *Service {
	logFile := path.Join(logDir, tag+".log")
	// The machine agent always starts with debug turned on.  The logger worker
	// will update this to the system logging environment as soon as it starts.
	conf := common.Conf{
		Desc: fmt.Sprintf("juju %s agent", tag),
		Limit: map[string]string{
			"nofile": fmt.Sprintf("%d %d", maxAgentFiles, maxAgentFiles),
		},
		Cmd: path.Join(toolsDir, "jujud") +
			" machine" +
			" --data-dir " + utils.ShQuote(dataDir) +
			" --machine-id " + machineId +
			" --debug",
		Out: logFile,
		Env: env,
	}
	svc := NewService(name, conf)
	return svc
}

// Service provides visibility into and control over an upstart service.
type Service struct {
	Name string
	Conf common.Conf
}

func NewService(name string, conf common.Conf) *Service {
	if conf.InitDir == "" {
		conf.InitDir = confDir
	}
	return &Service{Name: name, Conf: conf}
}

// confPath returns the path to the service's configuration file.
func (s *Service) confPath() string {
	return path.Join(s.Conf.InitDir, s.Name+".conf")
}

func (s *Service) UpdateConfig(conf common.Conf) {
	s.Conf = conf
}

// validate returns an error if the service is not adequately defined.
func (s *Service) validate() error {
	if s.Name == "" {
		return errors.New("missing Name")
	}
	if s.Conf.InitDir == "" {
		return errors.New("missing InitDir")
	}
	err := validate(s.Conf)
	return errors.Trace(err)
}

// render returns the upstart configuration for the service as a slice of bytes.
func (s *Service) render() ([]byte, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}

	data, err := Serialize(s.Name, s.Conf)
	return data, errors.Trace(err)
}

// Installed returns whether the service configuration exists in the
// init directory.
func (s *Service) Installed() bool {
	_, err := os.Stat(s.confPath())
	return err == nil
}

// Exists returns whether the service configuration exists in the
// init directory with the same content that this Service would have
// if installed.
func (s *Service) Exists() bool {
	// In any error case, we just say it doesn't exist with this configuration.
	// Subsequent calls into the Service will give the caller more useful errors.
	_, same, _, err := s.existsAndSame()
	if err != nil {
		return false
	}
	return same
}

func (s *Service) existsAndSame() (exists, same bool, conf []byte, err error) {
	expected, err := s.render()
	if err != nil {
		return false, false, nil, errors.Trace(err)
	}
	current, err := ioutil.ReadFile(s.confPath())
	if err != nil {
		if os.IsNotExist(err) {
			// no existing config
			return false, false, expected, nil
		}
		return false, false, nil, errors.Trace(err)
	}
	return true, bytes.Equal(current, expected), expected, nil
}

// Running returns true if the Service appears to be running.
func (s *Service) Running() bool {
	cmd := exec.Command("status", "--system", s.Name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return startedRE.Match(out)
}

// Start starts the service.
func (s *Service) Start() error {
	if s.Running() {
		return nil
	}
	err := runCommand("start", "--system", s.Name)
	if err != nil {
		// Double check to see if we were started before our command ran.
		if s.Running() {
			return nil
		}
	}
	return err
}

// Stop stops the service.
func (s *Service) Stop() error {
	if !s.Running() {
		return nil
	}
	return runCommand("stop", "--system", s.Name)
}

// StopAndRemove stops the service and then deletes the service
// configuration from the init directory.
func (s *Service) StopAndRemove() error {
	if !s.Installed() {
		return nil
	}
	if err := s.Stop(); err != nil {
		return err
	}
	return os.Remove(s.confPath())
}

// Remove deletes the service configuration from the init directory.
func (s *Service) Remove() error {
	if !s.Installed() {
		return nil
	}
	return os.Remove(s.confPath())
}

// Install installs and starts the service.
func (s *Service) Install() error {
	exists, same, conf, err := s.existsAndSame()
	if err != nil {
		return errors.Trace(err)
	}
	if same {
		return nil
	}
	if exists {
		if err := s.StopAndRemove(); err != nil {
			return errors.Annotate(err, "upstart: could not remove installed service")
		}

	}
	if err := ioutil.WriteFile(s.confPath(), conf, 0644); err != nil {
		return errors.Trace(err)
	}

	// On slower disks, upstart may take a short time to realise
	// that there is a service there.
	for attempt := InstallStartRetryAttempts.Start(); attempt.Next(); {
		if err = s.Start(); err == nil {
			break
		}
	}
	return err
}

// InstallCommands returns shell commands to install and start the service.
func (s *Service) InstallCommands() ([]string, error) {
	conf, err := s.render()
	if err != nil {
		return nil, err
	}
	return []string{
		fmt.Sprintf("cat >> %s << 'EOF'\n%sEOF\n", s.confPath(), conf),
		"start " + s.Name,
	}, nil
}
