// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

// RemoteApplicationSecretGrant represents a secret grant to a remote application through
// a relation.
type RemoteApplicationSecretGrant struct {

	// SecretID is the ID of the secret being granted.
	SecretID string

	// ApplicationName represents the name of the remote application receiving
	// the secret grant.
	ApplicationName string

	// RelationKey identifies the specific relation through which the grant has
	// been made.
	RelationKey string

	// ApplicationUUID is the UUID of the synthetic granted application
	ApplicationUUID string

	// RelationUUID is the UUID of the relation created for this remote
	// application consumer, through which the secret grant is made.
	RelationUUID string
}

// RemoteUnitConsumer contains details to import a granted secret
// consumer during migration. These are used to track down which unit has access
// to which revision of a granted secret.
type RemoteUnitConsumer struct {

	// SecretID is the ID of the secret being consumed
	SecretID string

	// Unit is the unit name of the consuming unit.
	Unit string

	// CurrentRevision is the revision of the secret that the unit is consuming.
	CurrentRevision int
}

// RemoteSecret contains details to import a remote secret during migration. These secrets
// are secrets in the consumer model that are consumed from remote applications
// through an offer relation.
type RemoteSecret struct {

	// SecretID is the ID of the remote secret
	SecretID string

	// SourceModelUUID is the UUID of the model offering the secret
	SourceModelUUID string

	// UnitUUID is the UUID of the unit consuming the secret
	UnitUUID string

	// Label is the label of the remote secret
	Label string

	// CurrentRevision is the consumed revision of the remote secret
	CurrentRevision int

	// LatestRevision is the latest revision of the remote secret
	LatestRevision int
}
