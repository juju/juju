// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"encoding/json"
	"time"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/secret"
	"github.com/juju/juju/domain/unitstate"
	"github.com/juju/juju/internal/errors"
)

// RelationSettings holds a relation uuid and local unit and
// app-level settings, represented by scalar types.
type RelationSettings struct {
	// RelationUUID is the UUID of the relation.
	RelationUUID relation.UUID

	// UnitSet represents settings of the unit to be set.
	UnitSet unitstate.Settings

	// UnitUnset represents the keys of settings for the unit to be unset.
	UnitUnset []string

	// ApplicationSet represents settings of the application to be set.
	ApplicationSet unitstate.Settings

	// ApplicationUnset represents the keys of settings for the application
	// to be unset.
	ApplicationUnset []string
}

// CommitHookUnitInfo contains the unit data loaded before commit-hook writes.
type CommitHookUnitInfo struct {
	// UnitUUID is the UUID of the unit these changes pertain to.
	UnitUUID string

	// UnitLife is the life of the unit.
	UnitLife life.Life

	// MachineUUID is the UUID of the unit's machine, if the unit is
	// machine-backed.
	MachineUUID *string
}

// CommitHookChangesArg contains data needed to commit a hook change
// represented by scalar types.
type CommitHookChangesArg struct {
	// UnitUUID is the uuid of the unit these changes pertain to.
	UnitUUID string

	// UnitLife is the expected life of the unit.
	UnitLife life.Life

	// MachineUUID is the UUID of the unit's machine, if the unit is
	// machine-backed.
	MachineUUID *string

	// RelationUnitSettings settings for the relation unit and application
	// which need to be updated.
	RelationSettings []RelationSettings

	// OpenPorts are GroupedPortRanges with ports to be opened.
	// PortRanges are grouped by relation endpoint name.
	OpenPorts network.GroupedPortRanges

	// ClosePorts are GroupedPortRanges with ports to be closed.
	// PortRanges are grouped by relation endpoint name.
	ClosePorts network.GroupedPortRanges

	// CharmState is key/value pairs for charm attributes.
	CharmState *map[string]string

	// AddStorage contains prepared unit storage adds to apply in the commit
	// hook transaction.
	AddStorage []unitstate.PreparedStorageAdd

	// SecretCreates contains charm secrets to create.
	SecretCreates []unitstate.CreateSecretArg

	// TrackLatestSecrets is a slice of URIs for which the latest revision should
	// be tracked.
	TrackLatestSecrets []string

	// SecretUpdates contains charm secrets to update.
	SecretUpdates []UpdateSecretArg

	// SecretGrants contains pre-resolved charm secret grant requests.
	SecretGrants []GrantSecretArg

	// SecretRevokes contains pre-resolved charm secret revoke requests.
	SecretRevokes []RevokeSecretArg

	// SecretDeletes contains charm secrets to delete, with pre-marshaled
	// removal job arguments.
	SecretDeletes []DeleteSecretArg
}

// DeleteSecretArg holds a secret deletion request ready for the state layer,
// with the JSON arg already serialized.
type DeleteSecretArg struct {
	// URI is the stringified secret URI identifying the secret to delete.
	URI string

	// ArgJSON is the pre-marshaled JSON for the removal job's arg column.
	// A nil value means no arg (delete all revisions).
	ArgJSON *string
}

// secretDeletionArg is the JSON payload stored in the removal table's arg
// column when scheduling a charm-secret deletion with specific revisions.
type secretDeletionArg struct {
	Revisions []int `json:"revisions"`
}

// GrantSecretArg holds a pre-resolved secret grant request ready for the
// state layer.
type GrantSecretArg struct {
	// SecretID is the secret identifier (the URI ID component).
	SecretID string

	// SubjectUUID is the resolved UUID of the entity gaining access.
	SubjectUUID string

	// SubjectTypeID is the type of the subject entity.
	SubjectTypeID secret.GrantSubjectType

	// ScopeUUID is the resolved UUID of the access scope entity.
	ScopeUUID string

	// ScopeTypeID is the type of the scope entity.
	ScopeTypeID secret.GrantScopeType

	// RoleID is the role being granted.
	RoleID secret.Role
}

// RevokeSecretArg holds a pre-resolved secret revoke request ready for the
// state layer.
type RevokeSecretArg struct {
	// SecretID is the secret identifier (the URI ID component).
	SecretID string

	// SubjectUUID is the resolved UUID of the entity losing access.
	SubjectUUID string

	// SubjectTypeID is the type of the subject entity.
	SubjectTypeID secret.GrantSubjectType
}

// UpdateSecretArg holds a pre-resolved secret update request ready for the
// state layer. All fields are scalar types suitable for transactional DB
// writes.
type UpdateSecretArg struct {
	// SecretID is the secret identifier (the URI ID component).
	SecretID string

	// RotatePolicy is the new rotation policy (nil if unchanged).
	RotatePolicy *secret.RotatePolicy

	// ExpireTime is the expiry time (nil if unchanged).
	ExpireTime *time.Time

	// Description is the new description (nil if unchanged).
	Description *string

	// Label is the new label (nil if unchanged).
	Label *string

	// Params are the backend-specific parameters.
	Params map[string]string

	// Data is the secret content (nil if using external backend).
	Data map[string]string

	// ValueRefBackendID is the backend ID for external storage (empty if internal).
	ValueRefBackendID string

	// ValueRefRevisionID is the revision ID from the external backend (empty if
	// internal).
	ValueRefRevisionID string

	// Checksum is the sha256 checksum of the secret content.
	Checksum string

	// RevisionUUID is the UUID for the new revision.
	RevisionUUID string

	// OwnerKind indicates whether the secret is owned by application or unit.
	OwnerKind secret.CharmSecretOwnerKind
}

// TransformCommitHookChangesArg takes a domain package CommitHookChangesArg
// struct and return an internal package CommitHookChangesArg struct. Does not
// include RelationSettings.
func TransformCommitHookChangesArg(
	in unitstate.CommitHookChangesArg, unitInfo CommitHookUnitInfo,
) (CommitHookChangesArg, error) {
	secretDeletes, err := transformSecretDeletes(in.SecretDeletes)
	if err != nil {
		return CommitHookChangesArg{}, err
	}

	secretRevokes, err := transformSecretRevokes(in.SecretRevokes)
	if err != nil {
		return CommitHookChangesArg{}, err
	}

	secretGrants, err := transformSecretGrants(in.SecretGrants)
	if err != nil {
		return CommitHookChangesArg{}, err
	}

	return CommitHookChangesArg{
		UnitUUID:           unitInfo.UnitUUID,
		UnitLife:           unitInfo.UnitLife,
		MachineUUID:        unitInfo.MachineUUID,
		OpenPorts:          in.OpenPorts,
		ClosePorts:         in.ClosePorts,
		CharmState:         in.CharmState,
		SecretCreates:      in.SecretCreates,
		TrackLatestSecrets: in.TrackLatestSecrets,
		SecretUpdates:      nil, // will be populated outside this function
		SecretGrants:       secretGrants,
		SecretRevokes:      secretRevokes,
		SecretDeletes:      secretDeletes,
		AddStorage:         in.AddStorage,
	}, nil
}

// transformSecretDeletes converts domain DeleteSecretArg values into internal
// DeleteSecretArg values, marshaling the revisions JSON outside the
// transaction.
func transformSecretDeletes(deletes []unitstate.DeleteSecretArg) ([]DeleteSecretArg, error) {
	if len(deletes) == 0 {
		return nil, nil
	}

	result := make([]DeleteSecretArg, 0, len(deletes))
	for i, del := range deletes {
		if del.URI == nil {
			return nil, errors.Errorf("delete secret arg at index %d has nil URI", i)
		}

		arg := DeleteSecretArg{
			URI: del.URI.String(),
		}
		if len(del.Revisions) > 0 {
			j, err := json.Marshal(secretDeletionArg{Revisions: del.Revisions})
			if err != nil {
				return nil, errors.Errorf("marshalling revisions arg for %q: %w", del.URI, err)
			}
			s := string(j)
			arg.ArgJSON = &s
		}
		result = append(result, arg)
	}
	return result, nil
}

// transformSecretRevokes converts domain RevokeSecretArg values into internal
// RevokeSecretArg values. The domain type carries pre-resolved UUIDs so the
// transformation is straightforward.
func transformSecretRevokes(revokes []unitstate.RevokeSecretArg) ([]RevokeSecretArg, error) {
	if len(revokes) == 0 {
		return nil, nil
	}

	result := make([]RevokeSecretArg, 0, len(revokes))
	for i, rev := range revokes {
		if rev.URI == nil {
			return nil, errors.Errorf("revoke secret arg at index %d has nil URI", i)
		}
		result = append(result, RevokeSecretArg{
			SecretID:      rev.URI.ID,
			SubjectUUID:   rev.SubjectUUID,
			SubjectTypeID: rev.SubjectTypeID,
		})
	}
	return result, nil
}

// transformSecretGrants converts domain GrantSecretArg values into internal
// GrantSecretArg values. The domain type carries pre-resolved UUIDs so the
// transformation is straightforward.
func transformSecretGrants(grants []unitstate.GrantSecretArg) ([]GrantSecretArg, error) {
	if len(grants) == 0 {
		return nil, nil
	}

	result := make([]GrantSecretArg, 0, len(grants))
	for i, g := range grants {
		if g.URI == nil {
			return nil, errors.Errorf("grant secret arg at index %d has nil URI", i)
		}
		result = append(result, GrantSecretArg{
			SecretID:      g.URI.ID,
			SubjectUUID:   g.SubjectUUID,
			SubjectTypeID: g.SubjectTypeID,
			ScopeUUID:     g.ScopeUUID,
			ScopeTypeID:   g.ScopeTypeID,
			RoleID:        g.RoleID,
		})
	}
	return result, nil
}
