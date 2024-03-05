// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"reflect"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/v4/shell"
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
	Env map[string]string

	// TODO(ericsnow) Add a Limit type, since the possible keys are known.

	// Limit holds the ulimit values that will be set when the command
	// runs. Each value will be used as both the soft and hard limit.
	// Valid values are integers or "infinity"
	Limit map[string]string

	// Timeout is how many seconds may pass before an exec call (e.g.
	// ExecStart) times out. Values less than or equal to 0 (the
	// default) are treated as though there is no timeout.
	Timeout int

	// ExecStart is the command (with arguments) that will be run. The
	// path to the executable must be absolute.
	// The command will be restarted if it exits with a non-zero exit code.
	ExecStart string

	// ExecStopPost is the command that will be run after the service stops.
	// The path to the executable must be absolute.
	ExecStopPost string

	// Logfile, if set, indicates where the service's output should be
	// written.
	Logfile string

	// TODO(ericsnow) Turn ExtraScript into ExecStartPre.

	// ExtraScript allows to insert script before command execution.
	ExtraScript string

	// ServiceBinary is the actual binary without any arguments.
	ServiceBinary string

	// ServiceArgs is a string array of unquoted arguments
	ServiceArgs []string
}

// IsZero determines whether or not the conf is a zero value.
func (c Conf) IsZero() bool {
	return reflect.DeepEqual(c, Conf{})
}

// Validate checks the conf's values for correctness.
func (c Conf) Validate(renderer shell.Renderer) error {
	if c.Desc == "" {
		return errors.New("missing Desc")
	}

	// Check the Exec* fields.
	if c.ExecStart == "" {
		return errors.New("missing ExecStart")
	}

	for field, cmd := range map[string]string{
		"ExecStart":    c.ExecStart,
		"ExecStopPost": c.ExecStopPost,
	} {
		if cmd == "" {
			continue
		}
		if err := c.checkExec(field, cmd, renderer); err != nil {
			return errors.Trace(err)
		}
	}

	for _, value := range c.Limit {
		switch value {
		case "unlimited", "infinity":
		default:
			_, err := strconv.Atoi(value)
			if err != nil {
				newErr := errors.NotValidf("limit must be \"infinity\", \"unlimited\", or an integer, %q", value)
				return errors.Wrap(err, newErr)
			}
		}
	}

	return nil
}

func (c Conf) checkExec(name, cmd string, renderer shell.Renderer) error {
	executable := strings.Fields(cmd)[0]
	executable = Unquote(executable)
	if !renderer.IsAbs(executable) {
		return errors.NotValidf("relative path in %s (%s)", name, executable)
	}
	return nil
}

// Unquote returns the string embedded between matching quotation marks.
// If there aren't any matching quotation marks then the string is
// returned as-is.
func Unquote(str string) string {
	if len(str) < 2 {
		return str
	}
	for _, quote := range []string{`'`, `"`} {
		if str[0:1] == quote && str[len(str)-1:] == quote {
			return strings.Trim(str, quote)
		}
	}
	return str
}
