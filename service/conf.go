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

	// TODO(ericsnow) Can we eliminate ExtraScript?

	// ExtraScript allows you to insert script before command execution.
	ExtraScript string
}

// Script composes ExtraScript and Cmd into a single script.
func (c Conf) Script() (string, error) {
	if len(c.Cmd) == 0 {
		return "", errors.New("missing Cmd")
	}

	script := c.Cmd
	if len(c.ExtraScript) > 0 {
		// TODO(ericsnow) Fix this on Windows.
		script = c.ExtraScript + "\n" + script
	}

	return script, nil
}

// normalize composes an initsystems.Conf from the service Conf. This
// may involve adjusting some values and validating others.
func (c Conf) normalize() (*initsystems.Conf, error) {
	if c.ExtraScript != "" {
		return nil, errors.NotSupportedf("ExtraScript")
	}
	normalConf := initsystems.Conf{
		Desc:  c.Desc,
		Cmd:   c.Cmd,
		Env:   c.Env,
		Limit: c.Limit,
		Out:   c.Out,
	}
	return &normalConf, nil
}
