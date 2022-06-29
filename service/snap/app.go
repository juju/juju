// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package snap

import (
	"fmt"

	"github.com/juju/errors"
)

// ConfinementPolicy describes the confinement policy for installing a given
// snap application.
type ConfinementPolicy string

const (
	// StrictPolicy confined snaps run in complete isolation, up to a minimal
	// access level that’s deemed always safe.
	StrictPolicy ConfinementPolicy = "strict"
	// ClassicPolicy allows access to your system’s resources in much the same
	// way traditional packages do
	ClassicPolicy ConfinementPolicy = "classic"
	// DevModePolicy is a special mode for snap creators and developers.
	// A devmode snap runs as a strictly confined snap with full access to
	// system resources, and produces debug output to identify unspecified
	// interfaces.
	DevModePolicy ConfinementPolicy = "devmode"
	// JailModePolicy enforces the confinement model for a snap to ensure that
	// the confinement is enforced.
	JailModePolicy ConfinementPolicy = "jailmode"
)

// Validate validates a given confinement policy to ensure it matches the ones we
// expect.
func (p ConfinementPolicy) Validate() error {
	switch p {
	case StrictPolicy, ClassicPolicy, DevModePolicy, JailModePolicy:
		return nil
	}
	return errors.NotValidf("%s confinement", p)
}

// String representation of the confinement policy.
func (p ConfinementPolicy) String() string {
	return string(p)
}

// App is a wrapper around a single snap
type App struct {
	name               string
	confinementPolicy  ConfinementPolicy
	channel            string
	backgroundServices []BackgroundService
	prerequisites      []Installable
}

// NewNamedApp creates a new application from a given name.
func NewNamedApp(name string) *App {
	return &App{
		name: name,
	}
}

// NewApp creates a application along with it's dependencies.
func NewApp(name string, channel string, policy ConfinementPolicy, services []BackgroundService, prerequisites []Installable) *App {
	return &App{
		name:               name,
		channel:            channel,
		confinementPolicy:  policy,
		backgroundServices: services,
		prerequisites:      prerequisites,
	}
}

// Validate will validate a given application for any potential issues.
func (a *App) Validate() error {
	if !snapNameRe.MatchString(a.name) {
		return errors.NotValidf("application %v", a.name)
	}

	if a.confinementPolicy != "" {
		if err := a.confinementPolicy.Validate(); err != nil {
			return errors.Trace(err)
		}
	}

	for _, backgroundService := range a.backgroundServices {
		err := backgroundService.Validate()
		if err != nil {
			return errors.Trace(err)
		}
	}

	for _, prerequisite := range a.prerequisites {
		err := prerequisite.Validate()
		if err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

// StartCommands returns a list if shell commands that should be executed (in order)
// to start App and its background services. executeable is a path to the snap
// executable. If the app has prerequisite applications defined, then take care to call
// StartCommands on those apps also.
func (a *App) StartCommands(executable string) []string {
	if len(a.backgroundServices) == 0 {
		return []string{fmt.Sprintf("%s start %s", executable, a.name)}
	}

	commands := make([]string, 0, len(a.backgroundServices))
	for _, backgroundService := range a.backgroundServices {
		enableFlag := ""
		if backgroundService.EnableAtStartup {
			enableFlag = " --enable "
		}

		command := fmt.Sprintf("%s start %s %s.%s", executable, enableFlag, a.name, backgroundService.Name)
		commands = append(commands, command)
	}
	return commands
}

// Install returns a way to install one application with all it's settings.
func (a *App) Install() []string {
	args := []string{
		"install",
	}
	if a.channel != "" {
		args = append(args, fmt.Sprintf("--channel=%s", a.channel))
	}
	if a.confinementPolicy != "" {
		args = append(args, fmt.Sprintf("--%s", a.confinementPolicy))
	}
	return append(args, a.name)
}

// Prerequisites defines a list of all the Prerequisites required before the
// application also needs to be installed.
func (a *App) Prerequisites() []Installable {
	return a.prerequisites
}

// BackgroundServices returns a list of background services that are
// required to be installed for the main application to run.
func (a *App) BackgroundServices() []BackgroundService {
	return a.backgroundServices
}

// Name returns the name of the application
func (a *App) Name() string {
	return a.name
}
