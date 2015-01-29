// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/errors"

	"github.com/juju/juju/service/initsystems"
)

// Conf is responsible for defining services. Its fields
// represent elements of a service configuration.
type Conf struct {
	initsystems.Conf

	// TODO(ericsnow) Can we eliminate ExtraScript?

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
