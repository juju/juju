// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"github.com/juju/errors"
)

type ControllerInfo struct {
	Controller
	Name string
}

// Write implements ControllerWriter.Write.
func (info *ControllerInfo) Write() error {
	if err := info.validate(); err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(UpdateController(info.Name, info.Controller))
}

// validate ensures that ControllerInfo has all necessary data.
func (info *ControllerInfo) validate() error {
	if info.Name == "" {
		return errors.NotValidf("missing name, controller info")
	}
	if info.ControllerUUID == "" {
		return errors.NotValidf("missing uuid, controller info")
	}
	if info.CACert == "" {
		return errors.NotValidf("missing ca-cert, controller info")
	}
	if len(info.Servers) == 0 {
		return errors.NotValidf("missing server host name(s), controller info")
	}
	if len(info.APIEndpoints) == 0 {
		return errors.NotValidf("missing api endpoint(s), controller info")
	}
	return nil
}
