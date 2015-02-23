// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/errors"

	"github.com/juju/juju/service/initsystems"
)

// Conf is the specification for an init system service configuration.
type Conf struct {
	initsystems.Conf

	// TODO(ericsnow) Eliminate ExtraScript.

	// ExtraScript allows you to insert script before command execution.
	ExtraScript string
}

// Script composes ExtraScript and Cmd into a single script.
func (c Conf) Script() (string, error) {
	if len(c.Cmd) == 0 {
		return "", errors.New("missing Cmd")
	}
	return c.script(), nil
}

func (c Conf) script() string {
	script := c.Cmd
	if len(c.ExtraScript) > 0 {
		// We trust that this will work on Windows.
		script = c.ExtraScript + "\n" + script
	}

	return script
}

// normalize composes an initsystems.Conf from the service Conf. This
// may involve adjusting some values and validating others.
func (c Conf) normalize() initsystems.Conf {
	script := c.script()

	env := c.Env
	if len(env) == 0 {
		env = nil
	}
	limit := c.Limit
	if len(limit) == 0 {
		limit = nil
	}

	return initsystems.Conf{
		Desc:  c.Desc,
		Cmd:   script,
		Env:   env,
		Limit: limit,
		Out:   c.Out,
	}
}
