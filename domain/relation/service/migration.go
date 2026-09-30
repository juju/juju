// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/logger"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/deployment/charm"
	domainmodelmigration "github.com/juju/juju/domain/modelmigration/modelmigration"
	"github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/domain/relation/internal"
	"github.com/juju/juju/internal/errors"
)

// MigrationState is the state required for migrating relations.
type MigrationState interface {
	// ImportPeerRelation establishes a peer relation on the endpoint passed as
	// argument. Used for migration import.
	ImportPeerRelation(
		ctx context.Context,
		uuid string,
		ep corerelation.EndpointIdentifier,
		id uint64,
		scope charm.RelationScope,
	) error

	// ImportRelation establishes a relation between two endpoints identified
	// by ep1 and ep2 and returns the relation UUID. Used for migration
	// import.
	ImportRelation(
		ctx context.Context,
		uuid string,
		ep1, ep2 corerelation.EndpointIdentifier,
		id uint64,
		scope charm.RelationScope,
	) error

	// GetApplicationUUIDByName returns the application UUID of the given application.
	GetApplicationUUIDByName(ctx context.Context, appName string) (application.UUID, error)

	// GetRelationEndpoints returns the endpoints of the relation with the
	// given UUID, ordered requirer first and provider second, matching the
	// key convention.
	//
	// The following error types can be expected to be returned:
	//   - [relationerrors.RelationNotFound] is returned if the relation UUID
	//     is not found.
	GetRelationEndpoints(ctx context.Context, relationUUID string) ([]relation.Endpoint, error)

	// SetRelationApplicationSettings records settings for a specific application
	// relation combination. Replaces all existing settings with the provided set.
	SetRelationApplicationSettings(
		ctx context.Context,
		relationUUID corerelation.UUID,
		applicationID application.UUID,
		settings map[string]string,
	) error

	// EnterScope indicates that the provided unit has joined the relation.
	// The unit's settings are written in the same transaction as the scope
	// membership, and only when the unit first enters its relation scope:
	// when the unit is already in scope, EnterScope reports
	// [relationerrors.RelationUnitAlreadyExists] and makes no changes to
	// state.
	//
	// When importing relations, zero-value storage arguments are expected: no
	// storage is provisioned for subordinate units during relation import.
	EnterScope(
		ctx context.Context,
		relationUUID corerelation.UUID,
		unitName unit.Name,
		settings map[string]string,
		subordinateStorageArgs internal.SubordinateUnitStorageArgs,
	) (internal.SubordinateUnitStatusHistoryData, error)

	// ExportRelations returns all relation information to be exported for the
	// model.
	ExportRelations(ctx context.Context) ([]relation.ExportRelation, error)
}

// MigrationService provides the API for importing relations.
type MigrationService struct {
	st     MigrationState
	logger logger.Logger
}

// NewMigrationService returns a new service reference wrapping the input state.
func NewMigrationService(st MigrationState, logger logger.Logger) *MigrationService {
	return &MigrationService{
		st:     st,
		logger: logger,
	}
}

// ImportRelations sets relations imported in migration.
func (s *MigrationService) ImportRelations(ctx context.Context, args relation.ImportRelationsArgs) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	for _, arg := range args {
		err := s.importRelation(ctx, arg)
		if err != nil {
			return errors.Capture(err)
		}

		for _, ep := range arg.Endpoints {
			err = s.importRelationEndpoint(ctx, arg.UUID, ep)
			if err != nil {
				return errors.Capture(err)
			}
		}
	}
	return nil
}

func (s *MigrationService) importRelation(ctx context.Context, arg relation.ImportRelationArg) error {
	if err := arg.UUID.Validate(); err != nil {
		return errors.Errorf("validating relation UUID for relation %d: %w", arg.ID, err)
	}
	if err := arg.Key.Validate(); err != nil {
		return errors.Errorf("validating relation key for relation %d: %w", arg.ID, err)
	}

	eps := arg.Key.EndpointIdentifiers()

	switch len(eps) {
	case 1:
		err := s.st.ImportPeerRelation(ctx, arg.UUID.String(), eps[0], uint64(arg.ID), arg.Scope)
		if err != nil {
			return errors.Errorf("importing peer relation %d by endpoint %q: %w", arg.ID, eps[0], err)
		}
	case 2:
		err := s.st.ImportRelation(ctx, arg.UUID.String(), eps[0], eps[1], uint64(arg.ID), arg.Scope)
		if err != nil {
			return errors.Errorf("importing relation %d between endpoints %q and %q: %w",
				arg.ID, eps[0], eps[1], err)
		}
	default:
		return errors.Errorf("unexpected number of endpoints %d for %q", len(eps), arg.Key)
	}
	return nil
}

// ImportConsumerProxyRelationSettingsAndUnits imports the application
// settings of each endpoint of the relations of remote application
// consumers, and the settings and scope membership of their units.
//
// Those relations are created by the cross model relation import, which
// must run first. Each relation is located by its UUID, being the relation
// token both models agreed on, and the key of the relation found is checked
// against the Key of the argument, so the data is only attached to the
// relation it was exported for. Every unit must belong to an application of
// the relation, which the state layer enforces.
//
// Importing the same relation twice leaves its state unchanged: the
// application settings are replaced with the same values, and a unit
// already in scope keeps the settings of its first entry.
func (s *MigrationService) ImportConsumerProxyRelationSettingsAndUnits(
	ctx context.Context,
	args relation.ImportRelationSettingsAndUnitsArgs,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	for _, arg := range args {
		if err := arg.UUID.Validate(); err != nil {
			return errors.Errorf("validating relation UUID: %w", err)
		}
		if err := arg.Key.Validate(); err != nil {
			return errors.Errorf("validating relation key: %w", err)
		}

		key, err := s.getRelationKeyByUUID(ctx, arg.UUID)
		if err != nil {
			return errors.Capture(err)
		}
		if !domainmodelmigration.RelationKeysEqual(key, arg.Key) {
			return errors.Errorf("relation %q has key %q, not %q", arg.UUID, key, arg.Key)
		}

		for _, ep := range arg.Endpoints {
			if err := s.importRelationEndpoint(ctx, arg.UUID, ep); err != nil {
				return errors.Errorf("importing %q endpoint data for relation %q: %w",
					ep.ApplicationName, arg.Key, err)
			}
		}
	}
	return nil
}

// getRelationKeyByUUID returns the key of the relation with the given UUID.
// A relation that does not exist is reported with the ordering the import
// depends on: the relations of remote application consumers are created by
// the cross model relation import, which must run before the relation
// import.
func (s *MigrationService) getRelationKeyByUUID(ctx context.Context, relUUID corerelation.UUID) (corerelation.Key, error) {
	endpoints, err := s.st.GetRelationEndpoints(ctx, relUUID.String())
	if errors.Is(err, relationerrors.RelationNotFound) {
		return nil, errors.Errorf("relation %q not found: relations of remote application consumers are created by the cross model relation import, which must run before the relation import", relUUID)
	} else if err != nil {
		return nil, errors.Capture(err)
	}
	return corerelation.Key(transform.Slice(endpoints, func(in relation.Endpoint) corerelation.EndpointIdentifier {
		return in.EndpointIdentifier()
	})), nil
}

// importRelationEndpoint imports the data of a single endpoint of a relation.
func (s *MigrationService) importRelationEndpoint(ctx context.Context, relUUID corerelation.UUID, ep relation.ImportEndpoint) error {
	appID, err := s.st.GetApplicationUUIDByName(ctx, ep.ApplicationName)
	if err != nil {
		return err
	}

	warningApp := func(key string) {
		s.logger.Warningf(ctx, "dropping empty value for key %q in application %q settings of relation %q",
			key, ep.ApplicationName, relUUID)
	}
	settings, err := settingsMap(warningApp, ep.ApplicationSettings)
	if err != nil {
		return err
	}
	err = s.st.SetRelationApplicationSettings(ctx, relUUID, appID, settings)
	if err != nil {
		return err
	}

	for unitName, unitSettings := range ep.UnitSettings {
		warningUnit := func(key string) {
			s.logger.Warningf(ctx, "dropping empty value for key %q in unit %s settings of relation %q",
				key, unitName, relUUID)
		}
		settings, err = settingsMap(warningUnit, unitSettings)
		if err != nil {
			return err
		}
		// Subordinate units are imported by the application domain before
		// relations, so entering scope here never creates a subordinate unit
		// that requires storage arguments.
		_, err = s.st.EnterScope(ctx, relUUID, unit.Name(unitName), settings, internal.SubordinateUnitStorageArgs{})
		if errors.Is(err, relationerrors.RelationUnitAlreadyExists) {
			// The unit is already in scope, and keeps the settings it was
			// given when it first entered, which EnterScope writes in the
			// same transaction as the scope membership. Nothing is left to
			// do, so the error is ignored: the same relation can be
			// imported twice in one run.
			continue
		} else if err != nil {
			return err
		}
	}
	return nil
}

// ExportRelations returns all relation information to be exported for the
// model.
func (s *MigrationService) ExportRelations(ctx context.Context) ([]relation.ExportRelation, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	relations, err := s.st.ExportRelations(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	// Generate the relation keys.
	for i, r := range relations {
		key := make(corerelation.Key, len(r.Endpoints))
		for j, ep := range r.Endpoints {
			key[j] = corerelation.EndpointIdentifier{
				ApplicationName: ep.ApplicationName,
				EndpointName:    ep.Name,
				Role:            ep.Role,
			}
		}
		relations[i].Key = key
	}

	return relations, nil
}

func settingsMap(warning func(string), in map[string]any) (map[string]string, error) {
	var errs error
	out := make(map[string]string)
	for k, v := range in {
		switch v.(type) {
		case string:
		default:
			errs = errors.Join(errs, errors.Errorf("%+v not a string", v))
			continue
		}
		if v == "" {
			warning(k)
			continue
		}
		out[k] = fmt.Sprintf("%v", v)
	}
	return out, errs
}
