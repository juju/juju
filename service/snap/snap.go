// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package snap is a minimal service.Service implementation, derived from the on service/upstart package.
package snap

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/v2"
	"github.com/juju/utils/v2/shell"

	"github.com/juju/juju/service/common"
	"github.com/juju/juju/service/systemd"
)

const (
	// Command is a path to the snap binary, or to one that can be detected by os.Exec
	Command = "snap"

	defaultConfinementPolicy = ""
	defaultChannel           = "stable"
)

var (
	logger = loggo.GetLogger("juju.service.snap")

	// snapNameRe is derived from https://github.com/snapcore/snapcraft/blob/a2ef08109d86259a0748446f41bce5205d00a922/schema/snapcraft.yaml#L81-106
	// but does not test for "--"
	snapNameRe = regexp.MustCompile("^[a-z0-9][a-z0-9-]{0,39}[^-]$")
)

// BackgroundService represents the a service that snaps define.
// For example, the multipass snap includes the libvirt-bin and multipassd background services.
type BackgroundService struct {
	// name is the name of the service, without the snap name.
	// For example , for the`juju-db.daemon` service, use the name `daemon`.
	Name string

	// enableAtStartup determines whether services provided
	// by the snap should be started with the `--enable` flag
	EnableAtStartup bool
}

// Validate checks that the construction parameters of
// backgroundService are valid. Successful validation
// returns nil.
func (backgroundService *BackgroundService) Validate() error {
	name := backgroundService.Name
	if name == "" {
		return errors.NotValidf("backgroundService.Name must be non-empty -")
	}

	if !snapNameRe.MatchString(name) {
		return errors.NotValidf("backgroundService.Name (%s) fails validation check -", name)
	}

	return nil
}

// App is a wrapper around a single snap
type App struct {
	Name               string
	ConfinementPolicy  string
	Channel            string
	BackgroundServices []BackgroundService
	Prerequisites      []App
}

func (a *App) Validate() error {
	var validationErrors = []error{}

	if !snapNameRe.MatchString(a.Name) {
		err := errors.NotValidf("app.Name")
		if err != nil {
			logger.Errorf("error detected in app.Name: %#v", a)
			validationErrors = append(validationErrors, err)
		}
	}

	for _, backgroundService := range a.BackgroundServices {
		err := backgroundService.Validate()
		if err != nil {
			validationErrors = append(validationErrors, err)
		}
	}

	for _, prerequisite := range a.Prerequisites {
		err := prerequisite.Validate()
		if err != nil {
			validationErrors = append(validationErrors, err)
		}
	}

	if len(validationErrors) == 0 {
		return nil
	}

	return errors.NotValidf("%v - snap.App", validationErrors)
}

// StartCommands returns a list if shell commands that should be executed (in order)
// to start App and its background services. executeable is a path to the snap
// executable. If the app has prerequisite applications defined, then take care to call
// StartCommands on those apps also.
func (a *App) StartCommands(executable string) []string {
	if len(a.BackgroundServices) == 0 {
		return []string{fmt.Sprintf("%s start %s", executable, a.Name)}
	}

	commands := make([]string, 0, len(a.BackgroundServices))
	for _, backgroundService := range a.BackgroundServices {
		enableFlag := ""
		if backgroundService.EnableAtStartup {
			enableFlag = " --enable "
		}

		command := fmt.Sprintf("%s start %s %s.%s", executable, enableFlag, a.Name, backgroundService.Name)
		commands = append(commands, command)
	}
	return commands
}

// IsRunning indicates whether Snap is currently running on the system.
// When the snap command (normally installed to /usr/bin/snap) cannot be
// detected, IsRunning returns (false, nil). Other errors result in (false, err).
func IsRunning() (bool, error) {
	if runtime.GOOS == "windows" {
		return false, nil
	}

	cmd := exec.Command(Command, "version")
	out, err := cmd.CombinedOutput()
	logger.Debugf("snap version output: %#v", string(out[:]))
	if err == nil {
		return true, nil
	}
	if common.IsCmdNotFoundErr(err) {
		return false, nil
	}

	return false, errors.Annotatef(err, "exec %q failed", Command)
}

// SetSnapConfig sets a snap's key to value.
func SetSnapConfig(snap string, key string, value string) error {
	if key == "" {
		return errors.NotValidf("key must not be empty")
	}

	cmd := exec.Command(Command, "set", snap, fmt.Sprintf("%s=%s", key, value))
	_, err := cmd.Output()
	if err != nil {
		return errors.Annotate(err, fmt.Sprintf("setting snap %s config %s to %s", snap, key, value))
	}

	return nil
}

// ListCommand returns a command that will be interpreted by a shell
// to produce a list of currently-installed services that are managed by snap.
func ListCommand() string {
	// filters the output from `snap list` to only be a newline-delimited list of snaps
	return Command + " services | tail -n +2 | cut -d ' ' -f1 | sort -u"
}

// ListServices returns a list of services that are being managed by snap.
func ListServices() ([]string, error) {
	fullCommand := strings.Fields(ListCommand())
	services, err := utils.RunCommand(fullCommand[0], fullCommand[1:]...)
	if err != nil {
		return []string{}, errors.Trace(err)
	}
	return strings.Split(services, "\n"), nil
}

// Service is a type for services that are being managed by snapd as snaps.
type Service struct {
	name           string
	scriptRenderer shell.Renderer
	executable     string
	app            App
	conf           common.Conf
	configDir      string
}

// NewService returns a new Service defined by `conf`, with the name `serviceName`.
// The Service abstracts service(s) provided by a snap.
//
// `serviceName` defaults to `snapName`. These two parameters are distinct to allow
// for a file path to provided as a `mainSnap`, implying that a local snap will be
// installed by snapd.
//
// If no BackgroundServices are provided, Service will wrap all of the snap's
// background services.
func NewService(mainSnap string, serviceName string, conf common.Conf, snapPath, configDir string, Channel string, ConfinementPolicy string, backgroundServices []BackgroundService, prerequisites []App) (Service, error) {
	if serviceName == "" {
		serviceName = mainSnap
	}
	if mainSnap == "" {
		return Service{}, errors.New("mainSnap must be provided")
	}
	app := App{
		Name:               mainSnap,
		ConfinementPolicy:  ConfinementPolicy,
		Channel:            Channel,
		BackgroundServices: backgroundServices,
		Prerequisites:      prerequisites,
	}
	err := app.Validate()
	if err != nil {
		return Service{}, errors.Trace(err)
	}

	svc := Service{
		name:           serviceName,
		scriptRenderer: &shell.BashRenderer{},
		executable:     snapPath,
		app:            app,
		conf:           conf,
		configDir:      configDir,
	}

	return svc, nil
}

func NewApp(name string) App {
	return App{
		Name:              name,
		ConfinementPolicy: defaultConfinementPolicy,
		Channel:           defaultChannel,
	}
}

// NewServiceFromName returns a service that manages all of a snap's
// services as if they were a single service. NewServiceFromName uses
// the name parameter to fetch and install a snap with a matching name, then uses
// default policies for the installation. To install a snap with --classic confinement,
// or via --edge, --candidate or --beta, then create the Service via another method.
func NewServiceFromName(name string, conf common.Conf) (Service, error) {
	Prerequisites := []App{}
	BackgroundServices := []BackgroundService{}
	Channel := defaultChannel
	ConfinementPolicy := defaultConfinementPolicy

	return NewService(name, name, conf, Command, systemd.EtcSystemdDir, Channel, ConfinementPolicy, BackgroundServices, Prerequisites)

}

// Validate validates that snap.Service has been correctly configured.
// Validate returns nil when successful and an error when successful.
func (s Service) Validate() error {
	var validationErrors = []error{}

	err := s.app.Validate()
	if err != nil {
		validationErrors = append(validationErrors, err)
	}

	for _, prerequisite := range s.app.Prerequisites {
		err = prerequisite.Validate()
		if err != nil {
			validationErrors = append(validationErrors, err)
		}
	}

	if len(validationErrors) == 0 {
		return nil
	}

	return errors.Errorf("snap.Service validation failed %v", validationErrors)
}

// Name returns the service's name. It should match snap's naming conventions,
// e.g. <snap> for all services provided by <snap> and `<snap>.<app>` for a specific service
// under the snap's control.For example, the `juju-db` snap provides a `daemon` service.
// Its name is `juju-db.daemon`.
//
// Name is part of the service.Service interface
func (s Service) Name() string {
	if s.name != "" {
		return s.name
	}
	return s.app.Name
}

// Conf returns the service's configuration.
//
// Conf is part of the service.Service interface.
func (s Service) Conf() common.Conf {
	return s.conf
}

// Running returns (true, nil) when snap indicates that service is currently active.
func (s Service) Running() (bool, error) {
	_, _, running, err := s.status()
	if err != nil {
		return false, errors.Trace(err)
	}
	return running, nil
}

// Exists is not implemented for snaps.
//
// Exists is part of the service.Service interface.
func (s Service) Exists() (bool, error) {
	return s.Installed()
}

// Install installs the snap and its background services.
//
// Install is part of the service.Service interface.
func (s Service) Install() error {
	commands, err := s.InstallCommands()
	if err != nil {
		return errors.Trace(err)
	}
	for _, cmd := range commands {
		if cmd == "" {
			continue
		}
		logger.Infof("command: %v", cmd)
		cmdParts := strings.Fields(cmd)
		executable := cmdParts[0]
		args := cmdParts[1:]
		out, err := utils.RunCommand(executable, args...)
		if err != nil {
			return errors.Annotatef(err, "output: %v", out)
		}

	}
	return nil
}

// Installed returns true if the service has been successfully installed.
//
// Installed is part of the service.Service interface.
func (s Service) Installed() (bool, error) {
	installed, _, _, err := s.status()
	if err != nil {
		return false, errors.Trace(err)
	}
	return installed, nil
}

// InstallCommands returns a slice of shell commands that is
// executed independently, in serial, by a shell. When the
// final command returns with a 0 exit code, the installation
// will be deemed to have been successful.
//
// InstallCommands is part of the service.Service interface
func (s Service) InstallCommands() ([]string, error) {
	commands := make([]string, 0, 1+len(s.app.Prerequisites))
	confinementPolicy := confimentParameterAsString(s.app.ConfinementPolicy)

	for _, prerequisite := range s.app.Prerequisites {
		command := fmt.Sprintf("%v install --channel=%v %v %v",
			s.executable,
			prerequisite.Channel,
			confinementPolicy,
			prerequisite.Name,
		)
		logger.Infof("preparing command: %v", command)
		commands = append(commands, command)
	}

	command := fmt.Sprintf("%v install --channel=%v %v %v",
		s.executable,
		s.app.Channel,
		confinementPolicy,
		s.app.Name,
	)
	logger.Infof("preparing command: %v", command)
	commands = append(commands, command)
	return commands, nil
}

// ConfigOverride writes a systemd override to enable the
// specified limits to be used by the snap.
func (s Service) ConfigOverride() error {
	if len(s.conf.Limit) == 0 {
		return nil
	}
	for _, backgroundService := range s.app.BackgroundServices {
		unitOptions := systemd.ServiceLimits(s.conf)
		data, err := ioutil.ReadAll(systemd.UnitSerialize(unitOptions))
		if err != nil {
			return errors.Trace(err)
		}
		overridesDir := fmt.Sprintf("%s/snap.%s.%s.service.d", s.configDir, s.name, backgroundService.Name)
		if err := os.MkdirAll(overridesDir, 0755); err != nil {
			return errors.Trace(err)
		}
		if err := ioutil.WriteFile(filepath.Join(overridesDir, "overrides.conf"), data, 0644); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func confimentParameterAsString(confinementPolicy string) string {
	if confinementPolicy != "" {
		return fmt.Sprintf("--%v", confinementPolicy)
	}
	return ""
}

// StartCommands returns a slice of strings. that are
// shell commands to be executed by a shell which start the service.
func (s Service) StartCommands() ([]string, error) {
	commands := make([]string, 0, 1+len(s.app.Prerequisites))
	for _, prerequisite := range s.app.Prerequisites {
		commands = append(commands, prerequisite.StartCommands(s.executable)...)
	}
	commands = append(commands, s.app.StartCommands(s.executable)...)
	return commands, nil
}

// status returns an interpreted output from the `snap services` command.
// For example, this output from `snap services juju-db.daemon`
//
//     Service                                Startup  Current
//     juju-db.daemon                         enabled  inactive
//
// returns this output from status
//
//     (true, true, false, nil)
func (s *Service) status() (isInstalled, enabledAtStartup, isCurrentlyActive bool, err error) {
	out, err := s.runCommand("services", s.Name())
	if err != nil {
		return false, false, false, errors.Trace(err)
	}
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, s.Name()) {
			continue
		}

		fields := strings.Fields(line)
		return true, fields[1] == "enabled", fields[2] == "active", nil
	}

	return false, false, false, nil
}

// Start starts the service, returning nil when successful.
// If the service is already running, Start does not restart it.
//
// Start is part of the service.ServiceActions interface
func (s Service) Start() error {
	running, err := s.Running()
	if err != nil {
		return errors.Trace(err)
	}
	if running {
		return nil
	}

	commands, err := s.StartCommands()
	if err != nil {
		return errors.Trace(err)
	}
	for _, command := range commands {
		commandParts := strings.Fields(command)
		out, err := utils.RunCommand(commandParts[0], commandParts[1:]...)
		if err != nil {
			if strings.Contains(out, "has no services") {
				continue
			}
			return errors.Annotatef(err, "%v -> %v", command, out)
		}
	}

	return nil
}

// Stop stops a running service. Returns nil when the underlying
// call to `snap stop <service-name>` exits with error code 0.
//
// Stop is part of the service.ServiceActions interface.
func (s Service) Stop() error {
	running, err := s.Running()
	if err != nil {
		return errors.Trace(err)
	}
	if !running {
		return nil
	}

	args := []string{"stop", s.Name()}
	return s.execThenExpect(args, "Stopped.")
}

// Remove uninstalls a service, . Returns nil when the underlying
// call to `snap remove <service-name>` exits with error code 0.
//
// Remove is part of the service.ServiceActions interface.
func (s Service) Remove() error {
	err := s.Stop()
	if err != nil {
		return errors.Trace(err)
	}

	args := []string{"remove", s.Name()}
	return s.execThenExpect(args, s.Name()+" removed")
}

// Restart restarts the service, or starts if it's not currently
// running.
//
// Restart is part of the service.RestartableService interface
func (s *Service) Restart() error {
	args := []string{"restart", s.Name()}
	return s.execThenExpect(args, "Restarted.")
}

// execThenExpect calls `snap <commandArgs>...` and then checks
// stdout against expectation and snap's exit code. When there's a
// mismatch or non-0 exit code, execThenExpect returns an error.
func (s *Service) execThenExpect(commandArgs []string, expectation string) error {
	out, err := s.runCommand(commandArgs...)
	if err != nil {
		return errors.Trace(err)
	}
	if !strings.Contains(out, expectation) {
		return errors.Annotatef(err, `expected "%s", got "%s"`, expectation, out)
	}
	return nil
}

func (s *Service) runCommand(args ...string) (string, error) {
	return utils.RunCommand(s.executable, args...)
}
