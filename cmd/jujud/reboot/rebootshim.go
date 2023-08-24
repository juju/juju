// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot

import (
	"fmt"
	"os"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/os/v2/series"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/internal/container"
	"github.com/juju/juju/internal/container/factory"
	"github.com/juju/juju/internal/service"
	"github.com/juju/juju/rpc/params"
)

// rebootWaiterShim wraps the functions required by RebootWaiter
// to facilitate mock testing.
type rebootWaiterShim struct {
}

// HostSeries returns the series of the current host.
func (r rebootWaiterShim) HostSeries() (string, error) {
	return series.HostSeries()
}

// ListServices returns a list of names of services running
// on the current host.
func (r rebootWaiterShim) ListServices() ([]string, error) {
	return service.ListServices()
}

// NewServiceReference returns a new juju service object.
func (r rebootWaiterShim) NewServiceReference(name string) (Service, error) {
	return service.NewServiceReference(name)
}

// NewContainerManager return an object implementing Manager.
func (r rebootWaiterShim) NewContainerManager(containerType instance.ContainerType, conf container.ManagerConfig) (Manager, error) {
	return factory.NewContainerManager(containerType, conf)
}

// ScheduleAction schedules the reboot action based on the
// current operating system.
func (r rebootWaiterShim) ScheduleAction(action params.RebootAction, after int) error {
	return scheduleAction(action, after)
}

// agentConfigShim wraps the method required by a Model in
// the RebootWaiter.
type agentConfigShim struct {
	aCfg agent.Config
}

// Model return an object implementing Model.
func (a *agentConfigShim) Model() Model {
	return a.aCfg.Model()
}

// TODO (tlm): This code has been moved across in the move to 3.0 removing
// Windows. However there are a number of things that can be fixed here for some
// easy wins.
// - Don't write out a script file. It introduces another failure point that we
// don't need to take on. We can just run the commands directly from the
// interpreter
//
// If we do decided to keep the script file:
// - Don't set the executable bit as we are giving the file directly to the
// interpreter.
// - Align the shabang line and the interpreter we use.

// scheduleAction will do a reboot or shutdown after given number of seconds
// this function executes the operating system's reboot binary with appropriate
// parameters to schedule the reboot
// If action is params.ShouldDoNothing, it will return immediately.
func scheduleAction(action params.RebootAction, after int) error {
	if action == params.ShouldDoNothing {
		return nil
	}
	args := []string{"shutdown"}
	switch action {
	case params.ShouldReboot:
		args = append(args, "-r")
	case params.ShouldShutdown:
		args = append(args, "-h")
	}
	args = append(args, "now")

	script, err := writeScript(args, after)
	if err != nil {
		return err
	}
	// Use the "nohup" command to run the reboot script without blocking.
	scheduled := []string{
		"nohup",
		"sh",
		script,
		"&",
	}
	return runCommand(scheduled)
}

func writeScript(args []string, after int) (string, error) {
	tpl := `#!/bin/bash
sleep %d
%s`
	script := fmt.Sprintf(tpl, after, strings.Join(args, " "))

	f, err := tmpFile()
	if err != nil {
		return "", errors.Trace(err)
	}
	defer func() { _ = f.Close() }()

	_, err = f.WriteString(script)
	if err != nil {
		return "", errors.Trace(err)
	}
	name := f.Name()
	err = os.Chmod(name, 0755)
	if err != nil {
		return "", errors.Trace(err)
	}
	return name, nil
}
