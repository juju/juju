// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"maps"

	"github.com/juju/collections/transform"

	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/trace"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/unitstate"
	"github.com/juju/juju/domain/unitstate/internal"
	"github.com/juju/juju/internal/errors"
)

// CommitHookChanges persists a set of changes after a hook successfully
// completes and executes them in a single transaction.
func (s *LeadershipService) CommitHookChanges(ctx context.Context, arg unitstate.CommitHookChangesArg) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	hasChanges, err := arg.ValidateAndHasChanges()
	if err != nil {
		return errors.Capture(err)
	}
	if !hasChanges {
		return nil
	}

	unitInfo, err := s.st.GetCommitHookUnitInfo(ctx, arg.UnitName.String())
	if err != nil {
		return errors.Capture(err)
	} else if unitInfo.UnitLife == life.Dead {
		return errors.Errorf(
			"unit %q is dead", arg.UnitName.String(),
		).Add(applicationerrors.UnitIsDead)
	}

	if unitInfo.UnitLife == life.Dying {
		// A dying unit cannot use new storage, so ignore storage add args.
		arg.AddStorage = nil
	}

	relationSettings, err := s.transformRelationSettings(ctx, arg.RelationSettings)
	if err != nil {
		return errors.Capture(err)
	}

	newArgs := internal.TransformCommitHookChangesArg(arg, unitInfo)
	newArgs.RelationSettings = relationSettings

	withCaveat, err := s.getManagementCaveat(arg)
	if err != nil {
		return err
	}
	return withCaveat(ctx, func(innerCtx context.Context) error {
		err := s.st.CommitHookChanges(innerCtx, newArgs)
		return errors.Capture(err)
	})
}

func (s *LeadershipService) getManagementCaveat(
	arg unitstate.CommitHookChangesArg,
) (func(context.Context, func(context.Context) error) error, error) {
	if arg.RequiresLeadership() {
		return func(ctx context.Context, fn func(context.Context) error) error {
			return s.leaderEnsurer.WithLeader(ctx, arg.UnitName.Application(), arg.UnitName.String(),
				func(ctx context.Context) error {
					return fn(ctx)
				},
			)
		}, nil
	}
	return func(ctx context.Context, fn func(context.Context) error) error {
		return fn(ctx)
	}, nil
}

func (s *Service) transformRelationSettings(
	ctx context.Context, in []unitstate.RelationSettings,
) ([]internal.RelationSettings, error) {
	return transform.SliceOrErr(in, func(in unitstate.RelationSettings) (internal.RelationSettings, error) {
		relationUUID, err := s.getRelationUUIDByKey(ctx, in.RelationKey)
		if err != nil {
			return internal.RelationSettings{}, errors.Capture(err)
		}
		settings := make(map[string]string, len(in.Settings))
		maps.Copy(settings, in.Settings)
		delete(settings, unitstate.IngressAddressKey)
		delete(settings, unitstate.EgressSubnetsKey)
		unitSet, unitUnset := parseForSetAndUnsetSettings(settings)
		appSet, appUnset := parseForSetAndUnsetSettings(in.ApplicationSettings)
		return internal.RelationSettings{
			RelationUUID:     relationUUID,
			UnitSet:          unitSet,
			UnitUnset:        unitUnset,
			ApplicationSet:   appSet,
			ApplicationUnset: appUnset,
		}, nil
	})
}

func parseForSetAndUnsetSettings(in unitstate.Settings) (unitstate.Settings, []string) {
	if len(in) == 0 {
		return nil, nil
	}

	// Determine the keys to set and unset.
	out := make(unitstate.Settings, 0)
	var unset []string
	for k, v := range in {
		if v == "" {
			unset = append(unset, k)
		} else {
			out[k] = v
		}
	}

	return out, unset
}

// getRelationUUIDByKey returns a relation UUID for the given Key.
func (s *Service) getRelationUUIDByKey(ctx context.Context, relationKey corerelation.Key) (corerelation.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	eids := relationKey.EndpointIdentifiers()
	var uuid corerelation.UUID
	var err error
	switch len(eids) {
	case 1:
		uuid, err = s.st.GetPeerRelationUUIDByEndpointIdentifiers(
			ctx,
			eids[0],
		)
		if err != nil {
			return "", errors.Errorf("getting peer relation by key: %w", err)
		}
		return uuid, nil
	case 2:
		uuid, err = s.st.GetRegularRelationUUIDByEndpointIdentifiers(
			ctx,
			eids[0],
			eids[1],
		)
		if err != nil {
			return "", errors.Errorf("getting regular relation by key: %w", err)
		}
		return uuid, nil
	default:
		return "", errors.Errorf("internal error: unexpected number of endpoints %d", len(eids))
	}
}
