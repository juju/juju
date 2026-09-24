// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/logger"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/deployment/charm"
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

	// SetRelationApplicationSettings records settings for a specific application
	// relation combination. Replaces all existing settings with the provided set.
	SetRelationApplicationSettings(
		ctx context.Context,
		relationUUID corerelation.UUID,
		applicationID application.UUID,
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

// ImportRelationData imports the endpoint data of relations that already exist
// in the model: the application settings of each endpoint, and the settings
// and relation scope membership of each of its units.
//
// It is for the relations that migration imports without this domain creating
// them, which are the relations of remote application consumers: those are
// created by the cross model relation import, because the offer connections it
// imports need them, and it hands their data over here because relation
// settings and unit scope membership belong to this domain.
//
// The relation is located by its UUID, which the caller must have resolved
// before calling. ID and Key are validated and reported, but play no part in
// locating anything, and Scope is not used, as nothing about the relation
// itself is written here. Both the relation and its endpoints must already
// exist, and every unit must belong to an application of the relation, which
// the state layer enforces.
//
// Re-importing a relation is not an error, so an import that failed part way
// through can be retried.
func (s *MigrationService) ImportRelationData(ctx context.Context, args relation.ImportRelationsArgs) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	for _, arg := range args {
		if err := arg.UUID.Validate(); err != nil {
			return errors.Errorf("validating relation UUID for relation %d: %w", arg.ID, err)
		}
		if err := arg.Key.Validate(); err != nil {
			return errors.Errorf("validating relation key for relation %d: %w", arg.ID, err)
		}

		for _, ep := range arg.Endpoints {
			if err := s.importRelationEndpoint(ctx, arg.UUID, ep); err != nil {
				return errors.Errorf("importing %q endpoint data for relation %d: %w",
					ep.ApplicationName, arg.ID, err)
			}
		}
	}
	return nil
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
		_, err = s.st.EnterScope(ctx, relUUID, unit.Name(unitName), settings)
		if errors.Is(err, relationerrors.RelationUnitAlreadyExists) {
			// The unit is already in scope, and keeps the settings it was given
			// when it first entered, which EnterScope writes in the same
			// transaction as the scope membership. Nothing is left to do, so
			// the error is ignored, which keeps an import that failed part way
			// through retriable.
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
