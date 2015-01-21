// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
)

// Conf is responsible for defining services. Its fields
// represent elements of a service configuration.
type Conf struct {
	// Desc is the init service's description.
	Desc string
	// Env holds the environment variables that will be set when the command runs.
	// Currently not used on Windows
	Env map[string]string
	// Limit holds the ulimit values that will be set when the command runs.
	// Currently not used on Windows
	Limit map[string]string
	// Cmd is the command (with arguments) that will be run.
	// The command will be restarted if it exits with a non-zero exit code.
	Cmd string
	// Out, if set, will redirect output to that path.
	Out string
	// InitDir is the folder in which the init script should be written
	// defaults to "/etc/init" on Ubuntu
	// Currently not used on Windows
	InitDir string
	// ExtraScript allows to insert script before command execution
	ExtraScript string
}

// Script composes ExtraScript and Cmd into a single script.
func (c Conf) Script() (string, error) {
	if len(c.Cmd) == 0 {
		return "", errors.New("missing Cmd")
	}
	if len(c.ExtraScript) == 0 {
		return c.Cmd, nil
	}
	// TODO(ericsnow) Fix this on Windows.
	return c.ExtraScript + "\n" + c.Cmd, nil
}
