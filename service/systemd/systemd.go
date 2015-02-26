// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package systemd

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"text/template"

	"github.com/juju/errors"
	"github.com/juju/utils"

	"github.com/juju/juju/service/common"
)

// This regexp will match the common output of a running service's
// `systemctl status unit.service` output.
var runningRegexp = regexp.MustCompile(`.*Active: active \(running\).*`)

// This regexp will match the common output of an enabled service's
// `systemctl status unit.service` output.
var enabledRegexp = regexp.MustCompile(`.*enabled.*`)

// The systemd service file directory with the uppermost priority.
var InitDir = "/etc/systemd/system"

// Service is the structure which provides a handle on a systemd service.
type Service struct {
	Name string
	Conf common.Conf
}

// New service returns a new *systemd.Service with the associated name and
// initial Conf. If no InitDir is provided, it defaults to InitDir.
func NewService(name string, conf common.Conf) *Service {
	if conf.InitDir == "" {
		conf.InitDir = InitDir
	}

	return &Service{Name: name, Conf: conf}
}

// UpdateConfig allows for the resetting of the Conf associated to s.
func (s *Service) UpdateConfig(conf common.Conf) {
	s.Conf = conf
}

// serviceName simply returns the fully qualified name of the service.
func (s *Service) serviceName() string {
	return s.Name + ".service"
}

// serviceFilePath returns the full path to the service file associated with s.
func (s *Service) serviceFilePath() string {
	return path.Join(s.Conf.InitDir, s.serviceName())
}

// extraScriptPath returns the full path to the file containing the ExtraScript
// of the service if it was provided in the service's Conf.
func (s *Service) extraScriptPath() string {
	return path.Join(s.Conf.InitDir, fmt.Sprintf("%s-extra.sh", s.Name))
}

// validate returns an error if the Service is not properly defined.
func (s *Service) validate() error {
	if s.Name == "" {
		return errors.New("missing Name")
	}
	if s.Conf.InitDir == "" {
		return errors.New("missing InitDir")
	}
	if s.Conf.Desc == "" {
		return errors.New("missing Desc")
	}
	if s.Conf.Cmd == "" {
		return errors.New("missing Cmd")
	}
	return nil
}

// render returns the systemd service file in slice of bytes form.
func (s *Service) render() ([]byte, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := serviceTemplate.Execute(&buf, s.Conf); err != nil {
		return nil, err
	}

	res := buf.String()

	// check for ExtraScript and apply its path (if applicable)
	if s.Conf.ExtraScript != "" {
		res = fmt.Sprintf(res, s.extraScriptPath())
	}

	return []byte(res), nil

}

// fileExistsAndMatches is a helper function which determines wether the file
// pointed to by path exists and if it matches the expected contents.
func fileExistsAndMatches(path string, expected []byte) (exists, matches bool, _ error) {
	var err error
	var found []byte

	if found, err = ioutil.ReadFile(path); err != nil {
		var reterr error
		if os.IsNotExist(err) {
			reterr = nil
		} else {
			reterr = err
		}

		return false, false, reterr
	}

	return true, bytes.Equal(found, expected), nil
}

// existsAndMatches is a helper function which determines whether a service file
// with the same name exists and whether it has the same contents.
// if applicable, it will also check for and compare the ExtraScript file.
func (s *Service) existsAndMatches() (exists, matches bool, _ error) {
	var err error
	var expected []byte
	var confExists, confMatches bool

	if expected, err = s.render(); err != nil {
		return false, false, errors.Trace(err)
	}

	// check for service file
	confExists, confMatches, err = fileExistsAndMatches(s.serviceFilePath(), expected)
	if err != nil {
		return false, false, errors.Trace(err)
	}

	// check for ExtraScript file
	if s.Conf.ExtraScript != "" {
		expected = []byte(fmt.Sprintf(extraScriptTemplate, s.Conf.ExtraScript))
		scriptExists, scriptMatches, err := fileExistsAndMatches(s.extraScriptPath(), expected)
		if err != nil {
			return false, false, errors.Trace(err)
		}

		return confExists && scriptExists, confMatches && scriptMatches, nil
	}

	return confExists, confMatches, nil
}

// runCommand is simply a variable for utils.RunCommand which was aliased for
// testing purposes
var runCommand = utils.RunCommand

// below lie some variables represing the go-systemd/dbus wrapper functions
// they were aliased for testing purposes
var list = listUnits
var reload = reloadDaemon
var enable = enableUnit
var disable = disableUnit
var start = startUnit
var stop = stopUnit

// enabled returns true if the service has been enabled in systemd.
// TODO (aznashwan): find a cleaner way to find the status of a unit
func (s *Service) enabled() bool {
	out, _ := runCommand("systemctl", "status", s.serviceName())

	return enabledRegexp.MatchString(out)
}

// Install properly places the service file of s in the InitDir, writes the
// ExtraScript file (if applicable) and starts the service through systemd.
// NOTE: a service will be enabled by default for it to count as installed.
// NOTE: the unit file daemon is automatically reloaded, making overriding an
// existing services possible by creating a new systemd.Service instance and
// Install()-ing it.
func (s *Service) Install() error {
	// check if the service is already installed
	exists, matches, err := s.existsAndMatches()
	if err != nil {
		return errors.Trace(err)
	}
	if matches && s.enabled() {
		return nil
	}
	if exists {
		if err := s.StopAndRemove(); err != nil {
			return errors.Trace(err)
		}
	}

	// write the service file to the InitDir
	contents, err := s.render()
	if err != nil {
		return errors.Trace(err)
	}
	if err := ioutil.WriteFile(s.serviceFilePath(), contents, 0644); err != nil {
		return errors.Trace(err)
	}

	// write the ExtraScript file (if applicable)
	if s.Conf.ExtraScript != "" {
		contents := fmt.Sprintf(extraScriptTemplate, s.Conf.ExtraScript)

		if err := ioutil.WriteFile(s.extraScriptPath(), []byte(contents), 0755); err != nil {
			return errors.Trace(err)
		}
	}

	// tell systemd to reload all unit files
	if err := reload(); err != nil {
		return errors.Trace(err)
	}

	// enable our service
	if err := enable(s.serviceFilePath()); err != nil {
		return errors.Trace(err)
	}

	// start the service to finish off the install
	return s.Start()
}

// Installed returns true if a service file was correctly set in the InitDir
// and was properly enabled.
func (s *Service) Installed() bool {
	exists, _, err := s.existsAndMatches()
	if err != nil {
		return false
	}

	return exists && s.enabled()
}

// Exists returns a boolean representing whether or not the exact service file
// which s would render to already exists and if it has been enabled.
func (s *Service) Exists() bool {
	_, matches, err := s.existsAndMatches()
	if err != nil {
		return false
	}

	return matches && s.enabled()
}

// Running returns a boolean of whether or not the service is actively running.
func (s *Service) Running() bool {
	// get all units currently present on the system
	units, err := list()
	if err != nil {
		return false
	}

	// iterate over all the units in search of one with the same
	// name as ours and check if it's running or not
	for _, unit := range units {
		if unit.Name == s.serviceName() {
			if unit.ActiveState == "active" {
				return true
			} else {
				return false
			}
		}
	}

	return false
}

// Start issues the command to systemd to immediately start the service.
func (s *Service) Start() error {
	if err := start(s.serviceName()); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// Stop issues the command to systemd to immediately stop the service.
func (s *Service) Stop() error {
	if err := stop(s.serviceName()); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// Remove disables the service and deletes the existing service file associated
// to s together with the ExtraScript file (if applicable).
func (s *Service) Remove() error {
	if err := disable(s.serviceFilePath()); err != nil {
		return errors.Trace(err)
	}

	// remove all files associated with the service
	os.Remove(s.serviceFilePath())

	if s.Conf.ExtraScript != "" {
		os.Remove(s.extraScriptPath())
	}

	// reload all unit files
	if err := reload(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// StopAndRemove stops the service and removes the service file together with
// the ExtraScript file (if applicable) from the InitDir.
func (s *Service) StopAndRemove() error {
	if err := s.Stop(); err != nil {
		return err
	}

	return s.Remove()
}

// InstallCommands returns the shell commands to install the service file,
// write the ExtraScript file (if applicable) and enable and run the service.
func (s *Service) InstallCommands() (cmds []string, _ error) {
	contents, err := s.render()
	if err != nil {
		return nil, err
	}

	if s.Conf.ExtraScript != "" {
		extraScript := fmt.Sprintf(extraScriptTemplate, s.Conf.ExtraScript)
		cmds = append(cmds, fmt.Sprintf("cat >> %s << 'EOF'\n%s\nEOF\n",
			s.extraScriptPath(), extraScript))
	}

	cmds = append(cmds, []string{
		fmt.Sprintf("cat >> %s << 'EOF'\n%s\nEOF\n", s.serviceFilePath(), contents),
		"systemctl daemon-reload",
		"systemctl enable " + s.Name,
		"systemctl start " + s.Name,
	}...)

	return cmds, nil
}

var serviceTemplate = template.Must(template.New("").Parse(`
[Unit]
Description={{.Desc}}
After=syslog.target
After=network.target
After=systemd-user-sessions.service

[Service]
Type=forking
{{range $k, $v := .Env}}Environment={{$k}}={{$v}}
{{end}}
{{if .ExtraScript}}ExecStartPre=%s{{end}}
ExecStart={{.Cmd}}
RemainAfterExit=yes
Restart=always
TimeoutSec=300

[Install]
WantedBy=default.target
`[1:]))

var extraScriptTemplate = `
#!/bin/sh

%s
`[1:]
