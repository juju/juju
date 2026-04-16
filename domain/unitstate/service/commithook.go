// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"maps"
	"slices"

	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/trace"
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

	unitUUID, err := s.st.GetUnitUUIDByName(ctx, arg.UnitName)
	if err != nil {
		return errors.Capture(err)
	}

	relationSettings, err := s.transformRelationSettings(ctx, arg.UpdatedRelationNetworkInfo, arg.RelationSettings)
	if err != nil {
		return errors.Capture(err)
	}

	newArgs := internal.TransformCommitHookChangesArg(arg, unitUUID)
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
	ctx context.Context, networkUpdate map[relation.UUID]unitstate.Settings, in []unitstate.RelationSettings,
) ([]internal.RelationSettings, error) {
	// If networkUpdates and relationSettings are empty, return early.
	if len(networkUpdate) == 0 && len(in) == 0 {
		return nil, nil
	}

	relationSettings, err := transform.SliceOrErr(in, func(in unitstate.RelationSettings) (internal.RelationSettings, error) {
		relationUUID, err := s.getRelationUUIDByKey(ctx, in.RelationKey)
		if err != nil {
			return internal.RelationSettings{}, errors.Capture(err)
		}
		settings := make(map[string]string, len(in.Settings))
		maps.Copy(settings, in.Settings)
		// The ingress address and egress subnets should be written only by juju.
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
	if err != nil {
		return nil, errors.Capture(err)
	}
	if len(networkUpdate) == 0 {
		return relationSettings, nil
	}

	// Build map for deduplication and merging.
	merged := make(map[relation.UUID]internal.RelationSettings)
	for _, rs := range relationSettings {
		merged[rs.RelationUUID] = rs
	}

	networkSettings := transform.MapToSlice(networkUpdate, func(key relation.UUID, value unitstate.Settings) []internal.RelationSettings {
		unitSet, unitUnset := parseForSetAndUnsetSettings(value)
		return []internal.RelationSettings{{
			RelationUUID: key,
			UnitSet:      unitSet,
			UnitUnset:    unitUnset,
		}}
	})
	for _, ns := range networkSettings {
		rs, exists := merged[ns.RelationUUID]
		if exists {
			// Merge UnitSet: networkSettings take precedence.
			if rs.UnitSet == nil {
				rs.UnitSet = make(unitstate.Settings)
			}
			maps.Copy(rs.UnitSet, ns.UnitSet)
			// Merge UnitUnset: union of both.
			unsetMap := make(map[string]struct{})
			for _, u := range rs.UnitUnset {
				unsetMap[u] = struct{}{}
			}
			for _, u := range ns.UnitUnset {
				unsetMap[u] = struct{}{}
			}
			unitUnset := make([]string, 0, len(unsetMap))
			for u := range unsetMap {
				unitUnset = append(unitUnset, u)
			}
			if len(unitUnset) == 0 {
				rs.UnitUnset = nil
			} else {
				rs.UnitUnset = unitUnset
			}
			merged[ns.RelationUUID] = rs
		} else {
			if len(ns.UnitUnset) == 0 {
				ns.UnitUnset = nil
			}
			merged[ns.RelationUUID] = ns
		}
	}

	return slices.Collect(maps.Values(merged)), nil
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
func (s *Service) getRelationUUIDByKey(ctx context.Context, relationKey relation.Key) (relation.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	eids := relationKey.EndpointIdentifiers()
	var uuid relation.UUID
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
