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
	logger.Debugf("validating snap app %q (path=%q, assertsPath=%q, confinement=%q, channel=%q)",
		a.name, a.path, a.assertsPath, a.confinementPolicy, a.channel)
	if !snapNameRe.MatchString(a.name) {
		logger.Warningf("snap app name %q does not match naming convention", a.name)
		return errors.NotValidf("application %v name", a.name)
	}

	if a.path != "" {
		logger.Debugf("snap app %q is a local snap, checking path %q", a.name, a.path)
		if _, err := os.Stat(a.path); err != nil {
			logger.Warningf("snap app %q local path %q not found: %v", a.name, a.path, err)
			return errors.NotFoundf("application %v path %v", a.name, a.path)
		}
		if a.assertsPath == "" {
			logger.Warningf("snap app %q is local but has no assert file specified", a.name)
			return errors.NotValidf("local snap %v requires an assert file", a.name)
		}
		if _, err := os.Stat(a.assertsPath); err != nil {
			logger.Warningf("snap app %q asserts path %q not found: %v", a.name, a.assertsPath, err)
			return errors.NotFoundf("application %v asserts path %v", a.name, a.assertsPath)
		}
		logger.Debugf("snap app %q local paths validated successfully", a.name)
	}

	if a.confinementPolicy != "" {
		logger.Debugf("validating confinement policy %q for snap app %q", a.confinementPolicy, a.name)
		if err := a.confinementPolicy.Validate(); err != nil {
			logger.Warningf("snap app %q confinement policy %q is invalid: %v", a.name, a.confinementPolicy, err)
			return errors.Trace(err)
		}
	}

	for _, backgroundService := range a.backgroundServices {
		logger.Debugf("validating background service %q for snap app %q", backgroundService.Name, a.name)
		err := backgroundService.Validate()
		if err != nil {
			logger.Warningf("background service %q validation failed for snap app %q: %v", backgroundService.Name, a.name, err)
			return errors.Trace(err)
		}
	}

	for _, prerequisite := range a.prerequisites {
		logger.Debugf("validating prerequisite %q for snap app %q", prerequisite.Name(), a.name)
		err := prerequisite.Validate()
		if err != nil {
			logger.Warningf("prerequisite %q validation failed for snap app %q: %v", prerequisite.Name(), a.name, err)
			return errors.Trace(err)
		}
	}

	logger.Debugf("snap app %q validation successful (%d background services, %d prerequisites)",
		a.name, len(a.backgroundServices), len(a.prerequisites))
	return nil
}

// StartCommands returns a list if shell commands that should be executed (in order)
// to start App and its background services. executeable is a path to the snap
// executable. If the app has prerequisite applications defined, then take care to call
// StartCommands on those apps also.
func (a *App) StartCommands(executable string) []string {
	logger.Debugf("generating start commands for snap app %q (executable=%q, background services=%d)",
		a.name, executable, len(a.backgroundServices))
	if len(a.backgroundServices) == 0 {
		cmd := fmt.Sprintf("%s start %s", executable, a.name)
		logger.Debugf("snap app %q has no background services, using single start command: %q", a.name, cmd)
		return []string{cmd}
	}

	commands := make([]string, 0, len(a.backgroundServices))
	for _, backgroundService := range a.backgroundServices {
		enableFlag := ""
		if backgroundService.EnableAtStartup {
			enableFlag = " --enable "
			logger.Debugf("background service %q.%q will be enabled at startup", a.name, backgroundService.Name)
		}

		command := fmt.Sprintf("%s start %s %s.%s", executable, enableFlag, a.name, backgroundService.Name)
		logger.Debugf("generated start command for %q.%q: %q", a.name, backgroundService.Name, command)
		commands = append(commands, command)
	}
	logger.Debugf("generated %d start commands for snap app %q", len(commands), a.name)
	return commands
}

// InstallArgs returns a way to install one application with all it's settings.
func (a *App) InstallArgs() []string {
	logger.Debugf("building install args for snap app %q (confinement=%q, path=%q, channel=%q)",
		a.name, a.confinementPolicy, a.path, a.channel)
	args := []string{
		"install",
	}
	if a.confinementPolicy != "" {
		logger.Debugf("snap app %q: adding confinement policy %q to install args", a.name, a.confinementPolicy)
		args = append(args, fmt.Sprintf("--%s", a.confinementPolicy))
	}
	if a.path != "" {
		// return early if this is a local snap, skipping over not-applicable
		// args such as channel
		args = append(args, a.path)
		logger.Debugf("snap app %q: local install args: %v", a.name, args)
		return args
	}
	if a.channel != "" {
		logger.Debugf("snap app %q: adding channel %q to install args", a.name, a.channel)
		args = append(args, fmt.Sprintf("--channel=%s", a.channel))
	}
	args = append(args, a.name)
	logger.Debugf("snap app %q: store install args: %v", a.name, args)
	return args
}

// AcknowledgeAssertsArgs returns args to acknowledge the asserts for the snap
// required to install this application. Returns nil if none are required.
func (a *App) AcknowledgeAssertsArgs() []string {
	if a.assertsPath == "" {
		logger.Debugf("snap app %q has no asserts path, no ack args needed", a.name)
		return nil
	}
	logger.Debugf("snap app %q: asserts acknowledgement args: ack %q", a.name, a.assertsPath)
	return []string{"ack", a.assertsPath}
}

// Prerequisites defines a list of all the Prerequisites required before the
// application also needs to be installed.
func (a *App) Prerequisites() []Installable {
	logger.Debugf("snap app %q has %d prerequisites", a.name, len(a.prerequisites))
	return a.prerequisites
}

// BackgroundServices returns a list of background services that are
// required to be installed for the main application to run.
func (a *App) BackgroundServices() []BackgroundService {
	logger.Debugf("snap app %q has %d background services", a.name, len(a.backgroundServices))
	return a.backgroundServices
}

// Name returns the name of the application
func (a *App) Name() string {
	return a.name
}
