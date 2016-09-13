// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/network"
)

// TargetInfo holds the details required to connect to a
// migration's target controller.
//
// TODO(mjs) - Note the similarity to api.Info. It would be nice to be
// able to use api.Info here but state can't import api and moving
// api.Info to live under the core package is too big a project to be
// done right now.
type TargetInfo struct {
	// ControllerTag holds tag for the target controller.
	ControllerTag names.ControllerTag

	// Addrs holds the addresses and ports of the target controller's
	// API servers.
	Addrs []string

	// CACert holds the CA certificate that will be used to validate
	// the target API server's certificate, in PEM format.
	CACert string

	// AuthTag holds the user tag to authenticate with to the target
	// controller.
	AuthTag names.UserTag

	// Password holds the password to use with AuthTag.
	Password string

	// Macaroons holds macaroons to use with AuthTag. At least one of
	// Password or Macaroons must be set.
	Macaroons []macaroon.Slice
}

// Validate returns an error if the TargetInfo contains bad data. Nil
// is returned otherwise.
func (info *TargetInfo) Validate() error {
	if !names.IsValidModel(info.ControllerTag.Id()) {
		return errors.NotValidf("ControllerTag")
	}

	if len(info.Addrs) < 1 {
		return errors.NotValidf("empty Addrs")
	}
	for _, addr := range info.Addrs {
		_, err := network.ParseHostPort(addr)
		if err != nil {
			return errors.NotValidf("%q in Addrs", addr)
		}
	}

	if info.CACert == "" {
		return errors.NotValidf("empty CACert")
	}

	if info.AuthTag.Id() == "" {
		return errors.NotValidf("empty AuthTag")
	}

	if info.Password == "" && len(info.Macaroons) == 0 {
		return errors.NotValidf("missing Password & Macaroons")
	}

	return nil
}
