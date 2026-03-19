// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package snap

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/retry"
	"github.com/juju/utils/v3"
	"github.com/juju/utils/v3/shell"

	"github.com/juju/juju/service/common"
	"github.com/juju/juju/service/systemd"
)

const (
	// Command is a path to the snap binary, or to one that can be detected by os.Exec
	Command = "snap"
)

var (
	logger = loggo.GetLogger("juju.service.snap")

	// snapNameRe is derived from https://github.com/snapcore/snapcraft/blob/a2ef08109d86259a0748446f41bce5205d00a922/schema/snapcraft.yaml#L81-106
	// but does not test for "--"
	snapNameRe = regexp.MustCompile("^[a-z0-9][a-z0-9-]{0,39}[^-]$")
)

// Runnable expects to be able to run a given command with a series of arguments
// and return the output and/or error from that executing command.
type Runnable interface {
	Execute(name string, args ...string) (string, error)
}

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
		return errors.NotValidf("empty background service name")
	}

	if !snapNameRe.MatchString(name) {
		return errors.NotValidf("background service name %q", name)
	}

	return nil
}

// SetSnapConfig sets a snap's key to value.
func SetSnapConfig(snap string, key string, value string) error {
	logger.Infof("setting snap %q config key %q to value %q", snap, key, value)
	if key == "" {
		logger.Warningf("set snap config called with empty key for snap %q", snap)
		return errors.NotValidf("key must not be empty")
	}

	cmd := exec.Command(Command, "set", snap, fmt.Sprintf("%s=%s", key, value))
	_, err := cmd.Output()
	if err != nil {
		logger.Errorf("failed to set snap %q config %q=%q: %v", snap, key, value, err)
		return errors.Annotate(err, fmt.Sprintf("setting snap %s config %s to %s", snap, key, value))
	}

	logger.Infof("successfully set snap %q config %q", snap, key)
	return nil
}

// Installable represents an installable snap.
type Installable interface {
	// Name returns the name of the application
	Name() string

	// InstallArgs returns args to install this application with all it's settings.
	InstallArgs() []string

	// AcknowledgeAssertsArgs returns args to acknowledge the asserts for the snap
	// required to install this application. Returns nil is none are required.
	AcknowledgeAssertsArgs() []string

	// Validate will validate a given application for any potential issues.
	Validate() error

	// StartCommands returns a list if shell commands that should be executed
	// (in order) to start App and its background services.
	StartCommands(executable string) []string

	// Prerequisites defines a list of all the Prerequisites required before the
	// application also needs to be installed.
	Prerequisites() []Installable

	// BackgroundServices returns a list of background services that are
	// required to be installed for the main application to run.
	BackgroundServices() []BackgroundService
}

// Service is a type for services that are being managed by snapd as snaps.
type Service struct {
	runnable       Runnable
	clock          clock.Clock
	name           string
	isLocal        bool
	scriptRenderer shell.Renderer
	executable     string
	app            Installable
	conf           common.Conf
	configDir      string
}

type ServiceConfig struct {
	// ServiceName is the name of this snap service.
	ServiceName string

	// SnapPath is an optional parameter that specifies the path on the filesystem
	// to install the snap from. If SnapPath is not provided, the snap will be
	// installed from the snap store.
	SnapPath string

	// SnapAssertsPath is an optional parameter that specifies the path on the
	// filesystem for any asserts that need to be acknowledged before installing.
	SnapAssertsPath string

	// Conf is responsible for defining services. Its fields
	// represent elements of a service configuration.
	Conf common.Conf

	// ConfigDir represents the directory path where the configuration files for
	// a service in a snap are located.
	ConfigDir string

	// Channel represents the channel to install the snap from, if we are installing
	// from the snap store.
	Channel string

	// SnapExecutable is the path where we can find the executable for snap itself.
	SnapExecutable string

	// ConfinementPolicy represents the confinement policy for installing a
	// snap application. It can have the following values: "strict", "classic",
	// "devmode", "jailmode". The "strict" policy enforces strict security
	// confinement, "classic" allows access to system resources, "devmode"
	// disables all confinement, and "jailmode" runs the snap in an isolated
	// environment.snap's confinement policy.
	ConfinementPolicy ConfinementPolicy

	// BackgroundServices represents a slice of background services required
	// by this snap service. When this service is started, these background
	// services will also be started.
	BackgroundServices []BackgroundService

	// Prerequisites represents a slice of prerequisite applications that need
	// to be installed to install this application.
	Prerequisites []Installable
}

// NewService returns a new Service defined by `conf`, with the name `serviceName`.
// The Service abstracts service(s) provided by a snap.
//
// If no BackgroundServices are provided, Service will wrap all of the snap's
// background services.
func NewService(config ServiceConfig) (Service, error) {
	logger.Infof("creating new snap service %q (path=%q, channel=%q, confinement=%q)",
		config.ServiceName, config.SnapPath, config.Channel, config.ConfinementPolicy)
	if config.ServiceName == "" {
		logger.Warningf("NewService called with empty ServiceName")
		return Service{}, errors.New("ServiceName must be provided")
	}
	app := &App{
		name:               config.ServiceName,
		path:               config.SnapPath,
		assertsPath:        config.SnapAssertsPath,
		confinementPolicy:  config.ConfinementPolicy,
		channel:            config.Channel,
		backgroundServices: config.BackgroundServices,
		prerequisites:      config.Prerequisites,
	}
	err := app.Validate()
	if err != nil {
		logger.Warningf("snap app validation failed for %q: %v", config.ServiceName, err)
		return Service{}, errors.Trace(err)
	}

	isLocal := config.SnapPath != ""
	logger.Debugf("snap service %q: isLocal=%v, configDir=%q, executable=%q",
		config.ServiceName, isLocal, config.ConfigDir, config.SnapExecutable)

	return Service{
		runnable:       defaultRunner{},
		clock:          clock.WallClock,
		name:           config.ServiceName,
		isLocal:        isLocal,
		scriptRenderer: &shell.BashRenderer{},
		executable:     config.SnapExecutable,
		app:            app,
		conf:           config.Conf,
		configDir:      config.ConfigDir,
	}, nil
}

// Validate validates that snap.Service has been correctly configured.
// Validate returns nil when successful and an error when successful.
func (s Service) Validate() error {
	logger.Debugf("validating snap service %q", s.name)
	if err := s.app.Validate(); err != nil {
		logger.Warningf("snap service %q app validation failed: %v", s.name, err)
		return errors.Trace(err)
	}

	for _, prerequisite := range s.app.Prerequisites() {
		if err := prerequisite.Validate(); err != nil {
			logger.Warningf(
				"snap service %q prerequisite %q validation failed: %v",
				s.name, prerequisite.Name(), err,
			)
			return errors.Trace(err)
		}
	}

	logger.Debugf("snap service %q validation successful", s.name)
	return nil
}

// Name returns the service's name. It should match snap's naming conventions,
// e.g. <snap> for all services provided by <snap> and `<snap>.<app>` for a specific service
// under the snap's control.For example, the `juju-db` snap provides a `daemon` service.
// Its name is `juju-db.daemon`.
func (s Service) Name() string {
	if s.name != "" {
		return s.name
	}
	return s.app.Name()
}

// IsLocal returns true if the snap is installed locally.
func (s Service) IsLocal() bool {
	return s.isLocal
}

// Running returns (true, nil) when snap indicates that service is currently active.
func (s Service) Running() (bool, error) {
	logger.Debugf("checking if snap service %q is running", s.name)
	_, _, running, err := s.status()
	if err != nil {
		logger.Warningf("failed to check running status for snap service %q: %v", s.name, err)
		return false, errors.Trace(err)
	}
	logger.Debugf("snap service %q running=%v", s.name, running)
	return running, nil
}

// Exists is not implemented for snaps.
func (s Service) Exists() (bool, error) {
	return false, errors.NotImplementedf("snap service Exists")
}

// Install installs the snap and its background services.
func (s Service) Install() error {
	logger.Infof("installing snap service %q (isLocal=%v)", s.name, s.isLocal)
	prerequisites := s.app.Prerequisites()
	logger.Infof("snap service %q has %d prereq(s) to install", s.name, len(prerequisites))
	for i, app := range prerequisites {
		logger.Infof("installing prerequisite %d/%d: %q", i+1, len(prerequisites), app.Name())

		out, err := s.installAppWithRetry(app)
		if err != nil {
			logger.Errorf(
				"failed to install prereq %q for snap service %q: %v (output: %v)",
				app.Name(), s.name, err, out,
			)
			return errors.Annotatef(err, "output: %v", out)
		}
		logger.Infof("successfully installed prerequisite %q", app.Name())
	}

	logger.Infof("installing snap app %q with args: %v", s.app.Name(), s.app.InstallArgs())
	out, err := s.installAppWithRetry(s.app)
	if err != nil {
		logger.Errorf("failed to install snap service %q: %v (output: %v)", s.name, err, out)
		return errors.Annotatef(err, "output: %v", out)
	}
	logger.Infof("successfully installed snap service %q", s.name)
	return nil
}

// installAppWithRetry installs the snap with retries. If the snap
// has asserts, it will acknowledge them before installing the snap.
func (s Service) installAppWithRetry(app Installable) (string, error) {
	ackAsserts := app.AcknowledgeAssertsArgs()
	if ackAsserts != nil {
		logger.Infof("acknowledging asserts for snap %q: %v", app.Name(), ackAsserts)
		_, err := s.runCommandWithRetry(ackAsserts...)
		if err != nil {
			logger.Errorf("failed to acknowledge asserts for snap %q: %v", app.Name(), err)
			return "", errors.Trace(err)
		}
		logger.Infof("successfully acknowledged asserts for snap %q", app.Name())
	} else {
		logger.Debugf("no asserts to acknowledge for snap %q", app.Name())
	}

	logger.Infof("running install command for snap %q with args: %v", app.Name(), app.InstallArgs())
	return s.runCommandWithRetry(app.InstallArgs()...)
}

// Installed returns true if the service has been successfully installed.
func (s Service) Installed() (bool, error) {
	logger.Debugf("checking if snap service %q is installed", s.name)
	installed, _, _, err := s.status()
	if err != nil {
		logger.Warningf("failed to check installed status for snap service %q: %v", s.name, err)
		return false, errors.Trace(err)
	}
	logger.Debugf("snap service %q installed=%v", s.name, installed)
	return installed, nil
}

// ConfigOverride writes a systemd override to enable the
// specified limits to be used by the snap.
func (s Service) ConfigOverride() error {
	logger.Debugf(
		"applying config overrides for snap service %q (limits count: %d)",
		s.name, len(s.conf.Limit),
	)
	if len(s.conf.Limit) == 0 {
		logger.Debugf("no config limits defined for snap service %q, skipping override", s.name)
		return nil
	}

	unitOptions := systemd.ServiceLimits(s.conf)
	data, err := io.ReadAll(systemd.UnitSerialize(unitOptions))
	if err != nil {
		logger.Errorf("failed to serialise systemd unit options for snap service %q: %v", s.name, err)
		return errors.Trace(err)
	}

	backgroundServices := s.app.BackgroundServices()
	logger.Infof(
		"writing config overrides for %d background services of snap %q",
		len(backgroundServices), s.name,
	)
	for _, backgroundService := range backgroundServices {
		overridesDir := fmt.Sprintf("%s/snap.%s.%s.service.d", s.configDir, s.name, backgroundService.Name)
		logger.Debugf(
			"creating overrides directory %q for background service %q",
			overridesDir, backgroundService.Name,
		)
		if err := os.MkdirAll(overridesDir, 0755); err != nil {
			logger.Errorf("failed to create overrides directory %q: %v", overridesDir, err)
			return errors.Trace(err)
		}
		overridePath := filepath.Join(overridesDir, "overrides.conf")
		logger.Debugf("writing overrides config to %q", overridePath)
		if err := os.WriteFile(overridePath, data, 0644); err != nil {
			logger.Errorf("failed to write overrides config to %q: %v", overridePath, err)
			return errors.Trace(err)
		}
	}
	logger.Infof("successfully applied config overrides for snap service %q", s.name)
	return nil
}

// StartCommands returns a slice of strings. that are
// shell commands to be executed by a shell which start the service.
func (s Service) StartCommands() ([]string, error) {
	deps := s.app.Prerequisites()
	logger.Debugf(
		"generating start commands for snap service %q (%d prerequisites)",
		s.name, len(deps),
	)
	commands := make([]string, 0, 1+len(deps))
	for _, prerequisite := range deps {
		cmds := prerequisite.StartCommands(s.executable)
		logger.Debugf("prerequisite %q start commands: %v", prerequisite.Name(), cmds)
		commands = append(commands, cmds...)
	}
	appCmds := s.app.StartCommands(s.executable)
	logger.Debugf("snap service %q start commands: %v", s.name, appCmds)
	commands = append(commands, appCmds...)
	logger.Debugf("total start commands for snap service %q: %v", s.name, commands)
	return commands, nil
}

// status returns an interpreted output from the `snap services` command.
// For example, this output from `snap services juju-db.daemon`
//
//	Service                                Startup  Current
//	juju-db.daemon                         enabled  inactive
//
// returns this output from status
//
//	(true, true, false, nil)
func (s *Service) status() (isInstalled, enabledAtStartup, isCurrentlyActive bool, err error) {
	logger.Debugf("querying status for snap service %q", s.Name())
	out, err := s.runCommand("services", s.Name())
	if err != nil {
		logger.Warningf("failed to query snap services for %q: %v", s.Name(), err)
		return false, false, false, errors.Trace(err)
	}
	logger.Debugf("snap services output for %q: %q", s.Name(), out)
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, s.Name()) {
			continue
		}

		fields := strings.Fields(line)
		installed := true
		enabled := fields[1] == "enabled"
		active := fields[2] == "active"
		logger.Debugf(
			"snap service %q status: installed=%v, enabledAtStartup=%v, active=%v",
			s.Name(), installed, enabled, active,
		)
		return installed, enabled, active, nil
	}

	logger.Debugf("snap service %q not found in services output", s.Name())
	return false, false, false, nil
}

// Start starts the service, returning nil when successful.
// If the service is already running, Start does not restart it.
func (s Service) Start() error {
	logger.Infof("starting snap service %q", s.name)
	running, err := s.Running()
	if err != nil {
		return errors.Trace(err)
	}
	if running {
		logger.Debugf("snap service %q is already running, skipping start", s.name)
		return nil
	}

	commands, err := s.StartCommands()
	if err != nil {
		logger.Errorf("failed to get start commands for snap service %q: %v", s.name, err)
		return errors.Trace(err)
	}
	logger.Infof("executing %d start commands for snap service %q", len(commands), s.name)
	for i, command := range commands {
		logger.Infof(
			"executing start command %d/%d for snap service %q: %q",
			i, len(commands), s.name, command,
		)
		commandParts := strings.Fields(command)
		out, err := utils.RunCommand(commandParts[0], commandParts[1:]...)
		if err != nil {
			if strings.Contains(out, "has no services") {
				logger.Debugf("snap %q has no services, skipping command %q", s.name, command)
				continue
			}
			logger.Errorf(
				"start command failed for snap service %q: %q -> %v (output: %v)",
				s.name, command, err, out,
			)
			return errors.Annotatef(err, "%v -> %v", command, out)
		}
		logger.Debugf(
			"start command %d/%d completed successfully for snap service %q (output: %q)",
			i, len(commands), s.name, out,
		)
	}

	logger.Infof("successfully started snap service %q", s.name)
	return nil
}

// Stop stops a running service. Returns nil when the underlying
// call to `snap stop <service-name>` exits with error code 0.
func (s Service) Stop() error {
	logger.Infof("stopping snap service %q", s.name)
	running, err := s.Running()
	if err != nil {
		return errors.Trace(err)
	}
	if !running {
		logger.Debugf("snap service %q is not running, skipping stop", s.name)
		return nil
	}

	args := []string{"stop", s.Name()}
	if err := s.execThenExpect(args, "Stopped."); err != nil {
		logger.Errorf("failed to stop snap service %q: %v", s.name, err)
		return err
	}
	logger.Infof("successfully stopped snap service %q", s.name)
	return nil
}

// Restart restarts the service, or starts if it's not currently
// running.
//
// Restart is part of the service.RestartableService interface
func (s Service) Restart() error {
	logger.Infof("restarting snap service %q", s.name)
	args := []string{"restart", s.Name()}
	if err := s.execThenExpect(args, "Restarted."); err != nil {
		logger.Errorf("failed to restart snap service %q: %v", s.name, err)
		return err
	}
	logger.Infof("successfully restarted snap service %q", s.name)
	return nil
}

// execThenExpect calls `snap <commandArgs>...` and then checks
// stdout against expectation and snap's exit code. When there's a
// mismatch or non-0 exit code, execThenExpect returns an error.
func (s Service) execThenExpect(commandArgs []string, expectation string) error {
	logger.Debugf("executing snap command %v, expecting %q", commandArgs, expectation)
	out, err := s.runCommand(commandArgs...)
	if err != nil {
		logger.Errorf("snap command %v failed: %v", commandArgs, err)
		return errors.Trace(err)
	}
	if !strings.Contains(out, expectation) {
		logger.Errorf(
			"snap command %v: expected %q in output, got %q",
			commandArgs, expectation, out,
		)
		return errors.Annotatef(err, `expected "%s", got "%s"`, expectation, out)
	}
	logger.Debugf("snap command %v output matched expectation %q", commandArgs, expectation)
	return nil
}

func (s Service) runCommand(args ...string) (string, error) {
	logger.Infof("running snap command: %v", args)
	return s.runnable.Execute(s.executable, args...)
}

func (s Service) runCommandWithRetry(args ...string) (res string, err error) {
	const delay = 5 * time.Second
	const attempts = 2
	logger.Debugf(
		"running snap command with retry: %v (delay=%v, attempts=%v)",
		args, delay, attempts,
	)
	attempt := 0
	if resErr := retry.Call(retry.CallArgs{
		Clock: s.clock,
		Func: func() error {
			attempt++
			logger.Debugf("snap command attempt %d: %v", attempt, args)
			res, err = s.runCommand(args...)
			if err != nil {
				logger.Warningf(
					"snap command attempt %d failed: %v (output: %q)",
					attempt, err, res,
				)
			}
			return errors.Trace(err)
		},
		Delay:    delay,
		Attempts: attempts,
	}); resErr != nil {
		logger.Errorf("snap command %v failed after %d attempts: %v", args, attempt, resErr)
		return "", errors.Trace(resErr)
	}

	logger.Debugf("snap command %v succeeded on attempt %d (output: %q)", args, attempt, res)
	// Named args are set via the retry.
	return
}

type defaultRunner struct{}

func (defaultRunner) Execute(name string, args ...string) (string, error) {
	return utils.RunCommand(name, args...)
}
