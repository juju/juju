// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"strings"

	"github.com/juju/errors"
)

// Conf is responsible for defining services. Its fields
// represent elements of a service configuration.
type Conf struct {
	// Desc is the init service's description.
	Desc string

	// Transient indicates whether or not the service is a one-off.
	Transient bool

	// AfterStopped is the name, if any, of another service. This
	// service will not start until after the other stops.
	AfterStopped string

	// Env holds the environment variables that will be set when the
	// command runs.
	// Currently not used on Windows.
	Env map[string]string

	// Limit holds the ulimit values that will be set when the command runs.
	// Currently not used on Windows.
	Limit map[string]string

	// ExecStart is the command (with arguments) that will be run. The
	// path to the executable must be absolute.
	// The command will be restarted if it exits with a non-zero exit code.
	ExecStart string

	// ExecStopPost is the command that will be run after the service stops.
	// The path to the executable must be absolute.
	ExecStopPost string

	// Output, if set, indicates where the service's output should be
	// sent. How that is interpreted depends on the init system. Some
	// accept paths to files while others only support certain identifiers.
	Output string

	// TODO(ericsnow) Eliminate InitDir.

	// InitDir is the folder in which the init script should be written
	// defaults to "/etc/init" on Ubuntu
	// Currently not used on Windows
	InitDir string

	// TODO(ericsnow) Turn ExtraScript into ExecStartPre.

	// ExtraScript allows to insert script before command execution.
	ExtraScript string
}

// Validate checks the conf's values for correctness.
func (c Conf) Validate() error {
	if c.Desc == "" {
		return errors.New("missing Desc")
	}

	if err := c.checkExecStart(); err != nil {
		return errors.Trace(err)
	}

	if err := c.checkExecStopPost(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func executable(cmd string) string {
	path := strings.Fields(cmd)[0]
	return unquote(path)
}

func unquote(str string) string {
	first, last := string(str[0]), string(str[len(str)-1])

	if first == `"` && last == `"` {
		return str[1 : len(str)-1]
	}

	if first == "'" && last == "'" {
		return str[1 : len(str)-1]
	}

	return str
}

func (c Conf) checkExecStart() error {
	if c.ExecStart == "" {
		return errors.New("missing ExecStart")
	}

	path := executable(c.ExecStart)
	if !strings.HasPrefix(path, "/") {
		return errors.NotValidf("relative path in ExecStart (%s)", path)
	}

	return nil
}

func (c Conf) checkExecStopPost() error {
	if c.ExecStopPost == "" {
		return nil
	}

	path := executable(c.ExecStopPost)
	if !strings.HasPrefix(path, "/") {
		return errors.NotValidf("relative path in ExecStopPost (%s)", path)
	}

	return nil
}
