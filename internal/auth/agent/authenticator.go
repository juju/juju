// Copyright 2025 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"context"

	coremachine "github.com/juju/juju/core/machine"
	coreunit "github.com/juju/juju/core/unit"
)

// AgentPasswordService defines the methods required to validate an agent
// password hash.
type AgentPasswordService interface {
	// MatchesMachinePasswordHashWithNonce checks if the password with a nonce
	// is valid or not.
	MatchesMachinePasswordHashWithNonce(context.Context, coremachine.Name, string, string) (bool, error)

	// MatchesControllerNodePasswordHash checks if the password is valid or
	// not against the password hash stored in the database for the controller
	// node.
	MatchesControllerNodePasswordHash(context.Context, string, string) (bool, error)

	// MatchesApplicationPasswordHash checks if the password is valid or not.
	MatchesApplicationPasswordHash(context.Context, string, string) (bool, error)

	// MatchModelPassword checks if the password hash is valid or not against the
	// password hash stored for the model's agent.
	MatchesModelPasswordHash(ctx context.Context, hash string) (bool, error)
}

type UnitAgentPasswordService interface {
	// GetUnitUUID retrieves the UUID of the unit with the given name.
	//
	// The following errors may be returned:
	// - [unit.InvalidUnitName] when the supplied unit name is invalid.
	// - [github.com/juju/juju/domain/application/errors.UnitNotFound] when no unit
	// for the given name exists in the model.
	GetUnitUUID(context.Context, coreunit.Name) (coreunit.UUID, error)

	// MatchesUnitPasswordHash checks if the password is valid or not against
	// the password hash stored in the database.
	//
	// The following errors may be returned:
	// - [passworderrors.EmptyPassword] when the supplied password is empty.
	// - [passworderrors.InvalidPassword] when the password length is greater than
	// the maximum supported.
	MatchesUnitPasswordHash(context.Context, coreunit.Name, string) (bool, error)
}

// UnitAgentAuthenticator provides an authenticator capable of authenticating
// unit agents within this controller.
type UnitAgentAuthenticator struct {
	service  UnitAgentPasswordService
	password string
	unitName coreunit.Name
}
