// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"github.com/juju/names/v6"
	"gopkg.in/macaroon.v2"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/errors"
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

	// ControllerAlias holds an optional alias for the target controller.
	ControllerAlias string

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

	// Token holds an optional token string to use for authentication
	// specifically with a JIMM controller.
	Token string
}

// Validate returns an error if the TargetInfo contains bad data. Nil
// is returned otherwise.
func (info *TargetInfo) Validate() error {
	if !names.IsValidModel(info.ControllerTag.Id()) {
		return errors.Errorf("ControllerTag %w", coreerrors.NotValid)
	}

	if len(info.Addrs) < 1 {
		return errors.Errorf("empty Addrs %w", coreerrors.NotValid)
	}
	for _, addr := range info.Addrs {
		_, err := network.ParseMachineHostPort(addr)
		if err != nil {
			return errors.Errorf("%q in Addrs %w", addr, coreerrors.NotValid)
		}
	}

	if info.AuthTag.Id() == "" && len(info.Macaroons) == 0 {
		return errors.Errorf("empty AuthTag %w", coreerrors.NotValid)
	}

	if info.Password == "" && len(info.Macaroons) == 0 && info.Token == "" {
		return errors.Errorf("missing Password, Macaroons or Token %w", coreerrors.NotValid)
	}

	return nil
}
