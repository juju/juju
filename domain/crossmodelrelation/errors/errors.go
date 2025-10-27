// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// RemoteApplicationNotFound describes an error that occurs when the remote
	// application being operated on does not exist.
	RemoteApplicationNotFound = errors.ConstError("remote application not found")

	// RemoteApplicationIsDead describes an error that occurs when the remote
	// application being operated on is dead.
	RemoteApplicationIsDead = errors.ConstError("remote application is dead")

	// RemoteApplicationHasRelations describes an error that occurs when an
	// operation on a remote application can't be performed because the remote
	// application has relations established.
	RemoteApplicationHasRelations = errors.ConstError("remote application has relations")

	// ApplicationNotRemote describes an error that occurs when the application
	// being operated on is not a remote application.
	ApplicationNotRemote = errors.ConstError("application not remote")

	// MissingEndpoints describes an error that occurs when not all of the
	// endpoints for the offer are found.
	MissingEndpoints = errors.ConstError("missing endpoints")

	// OfferNotFound describes an error that occurs when the offer
	// being operated on does not exist.
	OfferNotFound = errors.ConstError("offer not found")

	// OfferAlreadyConsumed describes an error that occurs when trying to
	// create an offer that already exists for the same UUID.
	OfferAlreadyConsumed = errors.ConstError("offer already consumed")

	// OfferAlreadyExists describes an error that occurs when trying to
	// create an offer that already exists.
	OfferAlreadyExists = errors.ConstError("offer already exists")

	// RemoteRelationAlreadyRegistered describes an error that occurs when
	// trying to register a remote relation that already exists.
	RemoteRelationAlreadyRegistered = errors.ConstError("remote relation already registered")

	// RemoteRelationNotFound describes an error that occurs when looking up a
	// remote relation by consumer relation UUID and it does not exist.
	RemoteRelationNotFound = errors.ConstError("remote relation not found")

	// RelationNotRemote indicates that the relation is not remote.
	RelationNotRemote = errors.ConstError("relation not remote")

	// MacaroonNotFound describes an error that occurs when looking up a
	// macaroon by relation UUID and it does not exist.
	MacaroonNotFound = errors.ConstError("macaroon not found")

	// SubnetNotInWhitelist describes an error that occurs when the provided
	// (ingress) subnet is not in the firewaller allowed whitelist.
	SubnetNotInWhitelist = errors.ConstError("subnet not in whitelist")
)
