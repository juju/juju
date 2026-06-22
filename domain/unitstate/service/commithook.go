// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"maps"
	"slices"

	"github.com/juju/collections/transform"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/relation"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/trace"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	domainsecret "github.com/juju/juju/domain/secret"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	"github.com/juju/juju/domain/unitstate"
	"github.com/juju/juju/domain/unitstate/internal"
	"github.com/juju/juju/internal/errors"
)

// CommitHookChanges persists a set of changes after a hook successfully
// completes and executes them in a single transaction.
func (s *LeadershipService) CommitHookChanges(ctx context.Context, arg unitstate.CommitHookChangesArg) (err error) {
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

	relationSettings, err := s.transformRelationSettings(ctx, arg.UpdatedRelationNetworkInfo, arg.RelationSettings)
	if err != nil {
		return errors.Capture(err)
	}

	// Pre-compute secret updates outside the transaction because
	// AddSecretBackendReference writes to the controller DB (a separate
	// DQLite database from the model DB), so it cannot run inside a model
	// DB transaction. If any part of the commit-hook transaction fails,
	// the deferred rollback function below calls all accumulated rollback
	// functions to undo the backend references. Rollback is best-effort:
	// failures are logged as warnings but the original error is returned,
	// since a stuck controller-DB reference can be cleaned up later while
	// losing the model-DB error would be unrecoverable.
	var rollbacks []func() error
	defer func() {
		if err != nil {
			s.rollbackAll(ctx, rollbacks)
		}
	}()

	// Pre-compute secret creates outside the transaction.
	secretCreates, createRollbacks, err := s.prepareSecretCreates(ctx, arg.SecretCreates, unitInfo)
	if err != nil {
		return errors.Capture(err)
	}
	rollbacks = append(rollbacks, createRollbacks...)

	// Pre-compute secret updates outside the transaction.
	secretUpdates, updateRollbacks, err := s.prepareSecretUpdates(ctx, arg.SecretUpdates)
	if err != nil {
		return errors.Capture(err)
	}
	rollbacks = append(rollbacks, updateRollbacks...)

	newArgs, err := internal.TransformCommitHookChangesArg(arg, unitInfo)
	if err != nil {
		return errors.Capture(err)
	}
	newArgs.RelationSettings = relationSettings
	newArgs.SecretCreates = secretCreates
	newArgs.SecretUpdates = secretUpdates

	withCaveat, err := s.getManagementCaveat(arg)
	if err != nil {
		return err
	}
	if err := withCaveat(ctx, func(innerCtx context.Context) error {
		err := s.st.CommitHookChanges(innerCtx, newArgs)
		return errors.Capture(err)
	}); err != nil {
		return err
	}

	return nil
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

// prepareSecretCreates pre-computes all data needed for secret creation
// outside the main transaction: generates revision UUIDs, timestamps, resolves
// owner UUIDs, and calls AddSecretBackendReference on the controller DB.
func (s *LeadershipService) prepareSecretCreates(
	ctx context.Context,
	creates []unitstate.CreateSecretArg,
	unitInfo internal.CommitHookUnitInfo,
) (_ []internal.CreateSecretArg, _ []func() error, err error) {
	if len(creates) == 0 {
		return nil, nil, nil
	}

	modelID, err := s.st.GetModelUUID(ctx)
	if err != nil {
		return nil, nil, errors.Errorf("getting model uuid: %w", err)
	}

	now := s.clock.Now()
	result := make([]internal.CreateSecretArg, 0, len(creates))
	rollbacks := make([]func() error, 0, len(creates))
	defer func() {
		if err != nil {
			s.rollbackAll(ctx, rollbacks)
		}
	}()

	for i, create := range creates {
		// Defensive check: the facade already enforces this, so this
		// guards against internal callers or future bugs. Not expected
		// to be hit in normal operation.
		if len(create.Data) > 0 && create.ValueRef != nil {
			return nil, nil, errors.Errorf("create[%d]: data and value ref are mutually exclusive", i)
		}

		revisionID, err := s.uuidGenerator()
		if err != nil {
			return nil, nil, errors.Errorf("generating revision UUID for create[%d]: %w", i, err)
		}

		p := domainsecret.UpsertSecretParams{
			Description:  create.Description,
			Label:        create.Label,
			ValueRef:     create.ValueRef,
			Checksum:     create.Checksum,
			CreateTime:   now,
			UpdateTime:   now,
			RevisionUUID: new(revisionID.String()),
		}

		if len(create.Data) > 0 {
			p.Data = make(coresecrets.SecretData)
			maps.Copy(p.Data, create.Data)
		}

		rotatePolicy := domainsecret.MarshallRotatePolicy(create.RotatePolicy)
		p.RotatePolicy = &rotatePolicy
		if create.RotatePolicy != nil && create.RotatePolicy.WillRotate() {
			p.NextRotateTime = create.RotatePolicy.NextRotateTime(now)
		}
		p.ExpireTime = create.ExpireTime

		// Resolve owner UUID.
		ownerKind := create.CharmOwner.Kind
		var ownerUUID string
		switch ownerKind {
		case domainsecret.ApplicationCharmSecretOwner:
			ownerUUID = unitInfo.ApplicationUUID
		case domainsecret.UnitCharmSecretOwner:
			ownerUUID = unitInfo.UnitUUID
		default:
			return nil, nil, errors.Errorf("unexpected owner kind %q for create[%d]", ownerKind, i)
		}

		// Add backend reference (controller DB, outside model txn).
		rollBack, err := s.secretBackendState.AddSecretBackendReference(
			ctx, p.ValueRef, coremodel.UUID(modelID), revisionID.String(), create.URI.ID)
		if err != nil {
			return nil, nil, errors.Errorf("adding backend reference for create[%d]: %w", i, err)
		}
		rollbacks = append(rollbacks, rollBack)

		var label string
		if create.Label != nil {
			label = *create.Label
		}

		result = append(result, internal.CreateSecretArg{
			SecretID:  create.URI.ID,
			Version:   create.Version,
			OwnerKind: ownerKind,
			OwnerUUID: ownerUUID,
			Label:     label,
			Params:    p,
		})
	}

	return result, rollbacks, nil
}

// prepareSecretUpdates pre-computes all data needed for secret updates
// outside the main transaction: generates revision UUIDs (if new data),
// timestamps, and calls AddSecretBackendReference on the controller DB.
func (s *LeadershipService) prepareSecretUpdates(
	ctx context.Context,
	updates []unitstate.UpdateSecretArg,
) (_ []internal.UpdateSecretArg, _ []func() error, err error) {
	if len(updates) == 0 {
		return nil, nil, nil
	}

	modelID, err := s.st.GetModelUUID(ctx)
	if err != nil {
		return nil, nil, errors.Errorf("getting model uuid: %w", err)
	}

	result := make([]internal.UpdateSecretArg, 0, len(updates))
	rollbacks := make([]func() error, 0, len(updates))
	// Roll back accumulated backend references on any error during
	// preparation. Failures are logged, not propagated — see rollbackAll.
	defer func() {
		if err != nil {
			s.rollbackAll(ctx, rollbacks)
		}
	}()

	for i, update := range updates {
		// Defensive check: the facade already enforces this, so this
		// guards against internal callers or future bugs. Not expected
		// to be hit in normal operation.
		if len(update.Data) > 0 && update.ValueRef != nil {
			return nil, nil, errors.Errorf("update[%d]: data and value ref are mutually exclusive", i)
		}

		arg := internal.UpdateSecretArg{
			SecretID:  update.URI.ID,
			Checksum:  update.Checksum,
			OwnerKind: update.OwnerKind,
		}

		if update.RotatePolicy != nil {
			p := domainsecret.MarshallRotatePolicy(update.RotatePolicy)
			arg.RotatePolicy = &p

			if update.RotatePolicy.WillRotate() {
				currentPolicy, err := s.st.GetSecretRotatePolicy(ctx, update.URI.ID)
				if errors.Is(err, secreterrors.SecretNotFound) {
					// SecretNotFound is expected when the secret is being created
					// in this same transaction or was concurrently deleted. In both
					// cases, we treat it as having RotateNever policy. This ensures
					// NextRotateTime is computed when updating to a policy that
					// rotates, implementing "last update wins" semantics. Transaction
					// isolation at the database layer handles actual concurrent deletes.
					currentPolicy = coresecrets.RotateNever
				} else if err != nil {
					return nil, nil, errors.Errorf("getting rotate policy for update[%d]: %w", i, err)
				}
				if update.RotatePolicy.LessThan(currentPolicy) {
					arg.NextRotateTime = update.RotatePolicy.NextRotateTime(s.clock.Now())
				}
			}
		}
		if update.ExpireTime != nil {
			arg.ExpireTime = update.ExpireTime
		}
		if update.Description != nil {
			arg.Description = update.Description
		}
		if update.Label != nil {
			arg.Label = update.Label
		}
		if len(update.Data) > 0 {
			arg.Data = make(map[string]string, len(update.Data))
			maps.Copy(arg.Data, update.Data)
		}
		if update.ValueRef != nil {
			arg.ValueRefBackendID = update.ValueRef.BackendID
			arg.ValueRefRevisionID = update.ValueRef.RevisionID
		}

		// Generate revision UUID and add backend reference.
		// We always create a new revision when data or a value ref
		// is present, even if the checksum matches the current
		// revision. This avoids a TOCTOU race between the pre-
		// compute phase and the model-DB transaction.
		if arg.ValueRefBackendID != "" || arg.ValueRefRevisionID != "" || len(arg.Data) != 0 {
			revisionID, err := s.uuidGenerator()
			if err != nil {
				return nil, nil, errors.Errorf("generating revision UUID for update[%d]: %w", i, err)
			}
			arg.RevisionUUID = revisionID.String()

			var valueRef *coresecrets.ValueRef
			if arg.ValueRefBackendID != "" {
				valueRef = &coresecrets.ValueRef{
					BackendID:  arg.ValueRefBackendID,
					RevisionID: arg.ValueRefRevisionID,
				}
			}

			rollBack, err := s.secretBackendState.AddSecretBackendReference(
				ctx, valueRef, coremodel.UUID(modelID), revisionID.String(), update.URI.ID)
			if err != nil {
				return nil, nil, errors.Errorf("adding backend reference for update[%d]: %w", i, err)
			}
			rollbacks = append(rollbacks, rollBack)
		}

		result = append(result, arg)
	}

	return result, rollbacks, nil
}

// rollbackAll executes all rollback functions, logging warnings on failure.
// Rollback failures are logged but not propagated because the original error
// that triggered the rollback is more important: losing the model-DB error
// would be unrecoverable, while a stuck controller-DB backend reference is
// cleaned up by the removal worker when the secret is eventually deleted.
func (s *LeadershipService) rollbackAll(ctx context.Context, rollbacks []func() error) {
	for _, rb := range rollbacks {
		if err := rb(); err != nil {
			s.logger.Warningf(ctx, "failed to roll back secret backend reference: %v", err)
		}
	}
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
