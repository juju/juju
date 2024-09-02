// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package snap

import (
	"fmt"
	"os"

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
	path               string
	assertsPath        string
	confinementPolicy  ConfinementPolicy
	channel            string
	backgroundServices []BackgroundService
	prerequisites      []Installable
}

// Validate will validate a given application for any potential issues.
func (a *App) Validate() error {
	if !snapNameRe.MatchString(a.name) {
		return errors.NotValidf("application %v name", a.name)
	}

	if a.path != "" {
		if _, err := os.Stat(a.path); err != nil {
			return errors.NotFoundf("application %v path %v", a.name, a.path)
		}
		if a.assertsPath == "" {
			return errors.NotValidf("local snap %v requires an assert file", a.name)
		}
		if _, err := os.Stat(a.assertsPath); err != nil {
			return errors.NotFoundf("application %v asserts path %v", a.name, a.assertsPath)
		}
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

// InstallArgs returns a way to install one application with all it's settings.
func (a *App) InstallArgs() []string {
	args := []string{
		"install",
	}
	if a.confinementPolicy != "" {
		args = append(args, fmt.Sprintf("--%s", a.confinementPolicy))
	}
	if a.path != "" {
		// return early if this is a local snap, skipping over not-applicable
		// args such as channel
		return append(args, a.path)
	}
	if a.channel != "" {
		args = append(args, fmt.Sprintf("--channel=%s", a.channel))
	}
	return append(args, a.name)
}

// AcknowledgeAssertsArgs returns args to acknowledge the asserts for the snap
// required to install this application. Returns nil if none are required.
func (a *App) AcknowledgeAssertsArgs() []string {
	if a.assertsPath == "" {
		return nil
	}
	return []string{"ack", a.assertsPath}
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
