// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/dbrootkeystore"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/user"
	userservice "github.com/juju/juju/domain/access/service"
)

// ControllerConfigService defines the methods required to get the controller
// configuration.
type ControllerConfigService interface {
	// ControllerConfig returns the config values for the controller.
	ControllerConfig(context.Context) (controller.Config, error)
}

// UserService defines the methods required to get user details.
type UserService interface {
	// GetUserByName returns the user with the given name.
	GetUserByName(context.Context, string) (user.User, error)
	// SetPasswordWithActivationKey will use the activation key from the user. To
	// then apply the payload password.
	SetPasswordWithActivationKey(ctx context.Context, name string, nonce, box []byte) (userservice.Sealer, error)
}

// MacaroonService defines the method required to manage macaroons.
type MacaroonService interface {
	dbrootkeystore.ContextBacking
	BakeryConfigService
}

// BakeryConfigService manages macaroon bakery config storage.
type BakeryConfigService interface {
	// GetOffersThirdPartyKey returns the key pair used with the cross-model
	// offers bakery.
	GetOffersThirdPartyKey(ctx context.Context) (*bakery.KeyPair, error)
	// GetExternalUsersThirdPartyKey returns the third party key pair used with
	// the external users bakery.
	GetExternalUsersThirdPartyKey(ctx context.Context) (*bakery.KeyPair, error)
}

// ModelInfoService provides access to information about the current model.
type ModelInfoService interface {
	// GetModelInfo returns the read-only information for the current model.
	GetModelInfo(ctx context.Context) (model.ReadOnlyModel, error)
}
