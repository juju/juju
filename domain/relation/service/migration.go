// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/application"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
)

// MigrationState is the state required for migrating relations.
type MigrationState interface {
	// GetPeerRelationUUIDByEndpointIdentifiers gets the UUID of a peer
	// relation specified by a single endpoint identifier.
	GetPeerRelationUUIDByEndpointIdentifiers(
		ctx context.Context,
		endpoint corerelation.EndpointIdentifier,
	) (corerelation.UUID, error)

	// ImportRelation establishes a relation between two endpoints identified
	// by ep1 and ep2 and returns the relation UUID. Used for migration
	// import.
	ImportRelation(
		ctx context.Context,
		ep1, ep2 corerelation.EndpointIdentifier,
		id uint64,
		scope charm.RelationScope,
	) (corerelation.UUID, error)

	// GetApplicationIDByName returns the application ID of the given application.
	GetApplicationIDByName(ctx context.Context, appName string) (application.ID, error)

	// SetRelationApplicationSettings records settings for a specific application
	// relation combination.
	SetRelationApplicationSettings(
		ctx context.Context,
		relationUUID corerelation.UUID,
		applicationID application.ID,
		settings map[string]string,
	) error

	// EnterScope indicates that the provided unit has joined the relation.
	// When the unit has already entered its relation scope, EnterScope will report
	// success but make no changes to state. The unit's settings are created or
	// overwritten in the relation according to the supplied map.
	EnterScope(
		ctx context.Context,
		relationUUID corerelation.UUID,
		unitName unit.Name,
		settings map[string]string,
	) error

	// DeleteImportedRelations deletes all imported relations in a model during
	// an import rollback.
	DeleteImportedRelations(
		ctx context.Context,
	) error

	// ExportRelations returns all relation information to be exported for the
	// model.
	ExportRelations(ctx context.Context) ([]relation.ExportRelation, error)
}

// MigrationService provides the API for importing relations.
type MigrationService struct {
	st MigrationState
}

// NewMigrationService returns a new service reference wrapping the input state.
func NewMigrationService(st MigrationState) *MigrationService {
	return &MigrationService{st: st}
}

// ImportRelations sets relations imported in migration.
func (s *MigrationService) ImportRelations(ctx context.Context, args relation.ImportRelationsArgs) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	for _, arg := range args {
		relUUID, err := s.importRelation(ctx, arg)
		if err != nil {
			return errors.Capture(err)
		}

		for _, ep := range arg.Endpoints {
			err = s.importRelationEndpoint(ctx, relUUID, ep)
			if err != nil {
				return errors.Capture(err)
			}
		}
	}
	return nil
}

func (s *MigrationService) importRelation(ctx context.Context, arg relation.ImportRelationArg) (corerelation.UUID, error) {
	var relUUID corerelation.UUID

	eps := arg.Key.EndpointIdentifiers()
	var err error

	switch len(eps) {
	case 1:
		// Peer relations are implicitly imported during migration of applications
		// during the call to CreateApplication.
		relUUID, err = s.st.GetPeerRelationUUIDByEndpointIdentifiers(ctx, eps[0])
		if err != nil {
			return relUUID, errors.Errorf("getting peer relation %d by endpoint %q: %w", arg.ID, eps[0], err)
		}
	case 2:
		relUUID, err = s.st.ImportRelation(ctx, eps[0], eps[1], uint64(arg.ID), arg.Scope)
		if err != nil {
			return relUUID, errors.Capture(err)
		}
	default:
		return relUUID, errors.Errorf("unexpected number of endpoints %d for %q", len(eps), arg.Key)
	}
	return relUUID, nil
}

func (s *MigrationService) importRelationEndpoint(ctx context.Context, relUUID corerelation.UUID, ep relation.ImportEndpoint) error {
	appID, err := s.st.GetApplicationIDByName(ctx, ep.ApplicationName)
	if err != nil {
		return err
	}

	settings, err := settingsMap(ep.ApplicationSettings)
	if err != nil {
		return err
	}
	err = s.st.SetRelationApplicationSettings(ctx, relUUID, appID, settings)
	if err != nil {
		return err
	}
	for unitName, unitSettings := range ep.UnitSettings {
		settings, err = settingsMap(unitSettings)
		if err != nil {
			return err
		}
		err = s.st.EnterScope(ctx, relUUID, unit.Name(unitName), settings)
		if err != nil {
			return err
		}
	}
	return nil
}

// DeleteImportedRelations deletes all imported relations in a model during
// an import rollback.
func (s *MigrationService) DeleteImportedRelations(
	ctx context.Context,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.st.DeleteImportedRelations(ctx)
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
		var eids []corerelation.EndpointIdentifier
		for _, ep := range r.Endpoints {
			eids = append(eids, corerelation.EndpointIdentifier{
				ApplicationName: ep.ApplicationName,
				EndpointName:    ep.Name,
				Role:            ep.Role,
			})
		}
		relations[i].Key, err = corerelation.NewKey(eids)
		if err != nil {
			return nil, errors.Errorf("generating relation key: %w", err)
		}
	}

	return relations, nil
}
