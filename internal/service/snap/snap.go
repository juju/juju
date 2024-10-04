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
	"github.com/juju/retry"
	"github.com/juju/utils/v4"
	"github.com/juju/utils/v4/shell"

	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/service/common"
	"github.com/juju/juju/internal/service/systemd"
)

const (
	// Command is a path to the snap binary, or to one that can be detected by os.Exec
	Command = "snap"
)

var (
	logger = internallogger.GetLogger("juju.service.snap")

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
	if config.ServiceName == "" {
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
		return Service{}, errors.Trace(err)
	}

	isLocal := config.SnapPath != ""

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
	if err := s.app.Validate(); err != nil {
		return errors.Trace(err)
	}

	for _, prerequisite := range s.app.Prerequisites() {
		if err := prerequisite.Validate(); err != nil {
			return errors.Trace(err)
		}
	}

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
	_, _, running, err := s.status()
	if err != nil {
		return false, errors.Trace(err)
	}
	return running, nil
}

// Exists is not implemented for snaps.
func (s Service) Exists() (bool, error) {
	return false, errors.NotImplementedf("snap service Exists")
}

// Install installs the snap and its background services.
func (s Service) Install() error {
	for _, app := range s.app.Prerequisites() {
		logger.Infof("command: %v", app)

		out, err := s.installAppWithRetry(app)
		if err != nil {
			return errors.Annotatef(err, "output: %v", out)
		}
	}

	out, err := s.installAppWithRetry(s.app)
	if err != nil {
		return errors.Annotatef(err, "output: %v", out)
	}
	return nil
}

// installAppWithRetry installs the snap with retries. If the snap
// has asserts, it will acknowledge them before installing the snap.
func (s Service) installAppWithRetry(app Installable) (string, error) {
	ackAsserts := app.AcknowledgeAssertsArgs()
	if ackAsserts != nil {
		_, err := s.runCommandWithRetry(ackAsserts...)
		if err != nil {
			return "", errors.Trace(err)
		}
	}

	return s.runCommandWithRetry(app.InstallArgs()...)
}

// Installed returns true if the service has been successfully installed.
func (s Service) Installed() (bool, error) {
	installed, _, _, err := s.status()
	if err != nil {
		return false, errors.Trace(err)
	}
	return installed, nil
}

// ConfigOverride writes a systemd override to enable the
// specified limits to be used by the snap.
func (s Service) ConfigOverride() error {
	if len(s.conf.Limit) == 0 {
		return nil
	}

	unitOptions := systemd.ServiceLimits(s.conf)
	data, err := io.ReadAll(systemd.UnitSerialize(unitOptions))
	if err != nil {
		return errors.Trace(err)
	}

	for _, backgroundService := range s.app.BackgroundServices() {
		overridesDir := fmt.Sprintf("%s/snap.%s.%s.service.d", s.configDir, s.name, backgroundService.Name)
		if err := os.MkdirAll(overridesDir, 0755); err != nil {
			return errors.Trace(err)
		}
		if err := os.WriteFile(filepath.Join(overridesDir, "overrides.conf"), data, 0644); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// StartCommands returns a slice of strings. that are
// shell commands to be executed by a shell which start the service.
func (s Service) StartCommands() ([]string, error) {
	deps := s.app.Prerequisites()
	commands := make([]string, 0, 1+len(deps))
	for _, prerequisite := range deps {
		commands = append(commands, prerequisite.StartCommands(s.executable)...)
	}
	return append(commands, s.app.StartCommands(s.executable)...), nil
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

// Restart restarts the service, or starts if it's not currently
// running.
//
// Restart is part of the service.RestartableService interface
func (s Service) Restart() error {
	args := []string{"restart", s.Name()}
	return s.execThenExpect(args, "Restarted.")
}

// execThenExpect calls `snap <commandArgs>...` and then checks
// stdout against expectation and snap's exit code. When there's a
// mismatch or non-0 exit code, execThenExpect returns an error.
func (s Service) execThenExpect(commandArgs []string, expectation string) error {
	out, err := s.runCommand(commandArgs...)
	if err != nil {
		return errors.Trace(err)
	}
	if !strings.Contains(out, expectation) {
		return errors.Annotatef(err, `expected "%s", got "%s"`, expectation, out)
	}
	return nil
}

func (s Service) runCommand(args ...string) (string, error) {
	logger.Infof("running snap command: %v", args)
	return s.runnable.Execute(s.executable, args...)
}

func (s Service) runCommandWithRetry(args ...string) (res string, err error) {
	if resErr := retry.Call(retry.CallArgs{
		Clock: s.clock,
		Func: func() error {
			res, err = s.runCommand(args...)
			return errors.Trace(err)
		},
		Delay:    5 * time.Second,
		Attempts: 2,
	}); resErr != nil {
		return "", errors.Trace(resErr)
	}

	// Named args are set via the retry.
	return
}

type defaultRunner struct{}

func (defaultRunner) Execute(name string, args ...string) (string, error) {
	return utils.RunCommand(name, args...)
}
