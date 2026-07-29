// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"sort"

	"github.com/juju/collections/set"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/trace"
	modelmigrationinternal "github.com/juju/juju/domain/modelmigration/internal"
	"github.com/juju/juju/internal/errors"
)

// ValidateImportedModel runs read-only consistency checks over an imported,
// still-gated model, used by the target VALIDATION phase (via ActivateImport)
// before activation clears the model_migrating gate. It performs no writes; a
// non-nil error means the imported model must not be activated.
//
// It checks the invariants the model schema cannot enforce with foreign keys
// alone:
//
//   - every secret backend referenced by the model's external secret value
//     refs exists on this controller (an un-rewritten source-controller-local
//     backend UUID would not),
//   - every externally backed secret revision has a matching
//     secret_backend_reference row on this controller (so external content
//     resolves and backend ref-counting stays correct after import), and
//   - every unit of an application participating in a relation that is expected
//     to be in the relation's scope has the corresponding relation-unit row
//     (schema foreign keys do not enforce this membership invariant, and an
//     import that omits the row would otherwise activate a structurally
//     incomplete relation).
//
// Machine, instance and credential validation lives in [Service.CheckMachines],
// which the master worker drives during the same VALIDATION phase.
func (s *Service) ValidateImportedModel(ctx context.Context) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := s.validateSecretBackendsExist(ctx); err != nil {
		return err
	}
	if err := s.validateSecretBackendReferences(ctx); err != nil {
		return err
	}
	return s.validateRelationConsistency(ctx)
}

// validateRelationConsistency fails if any alive unit belonging to an
// application that participates in an alive relation lacks the corresponding
// relation-unit row. Schema foreign keys do not enforce this invariant, and an
// import that omits the row would otherwise activate a structurally incomplete
// relation. Dying or dead relations and units are excluded: they legitimately
// leave relation scope before removal.
//
// The invariant is deliberately strict, and matches what 3.6 enforced in its
// source precheck (migration.SourcePrecheck -> checkRelations): a migration
// only runs once every agent is idle, so a unit that has not yet entered the
// scope of one of its application's relations means the model is not settled
// and must not be migrated. Units that can never be in scope are skipped, see
// [unitExpectedInRelationScope].
func (s *Service) validateRelationConsistency(ctx context.Context) error {
	relations, err := s.modelState.GetRelationValidationData(ctx)
	if err != nil {
		return errors.Errorf("reading relations of imported model: %w", err)
	}
	if len(relations) == 0 {
		return nil
	}

	appUnits, err := s.modelState.GetApplicationUnitNames(ctx)
	if err != nil {
		return errors.Errorf("reading application units of imported model: %w", err)
	}
	relationUnits, err := s.modelState.GetRelationUnitsByApplication(ctx)
	if err != nil {
		return errors.Errorf("reading relation units of imported model: %w", err)
	}
	principalApps, err := s.modelState.GetSubordinateUnitPrincipals(ctx)
	if err != nil {
		return errors.Errorf("reading subordinate unit principals of imported model: %w", err)
	}

	for _, relation := range relations {
		relationApps := set.NewStrings()
		subordinateApps := set.NewStrings()
		for _, endpoint := range relation.Endpoints {
			relationApps.Add(endpoint.ApplicationName)
			if endpoint.Subordinate {
				subordinateApps.Add(endpoint.ApplicationName)
			}
		}

		for _, endpoint := range relation.Endpoints {
			unitsInScope := set.NewStrings(relationUnits[relation.UUID][endpoint.ApplicationName]...)
			for _, unitName := range appUnits[endpoint.ApplicationName] {
				if !unitExpectedInRelationScope(endpoint, relationApps, subordinateApps, principalApps[unitName]) {
					continue
				}
				if !unitsInScope.Contains(unitName) {
					return errors.Errorf("unit %s hasn't joined relation %q yet", unitName, relation.Key)
				}
			}
		}
	}
	return nil
}

// unitExpectedInRelationScope reports whether a unit of the endpoint's
// application should have a relation-unit row for the relation. principalApp is
// the application of the unit's principal, empty when the unit is itself a
// principal.
//
// A container-scoped endpoint only holds subordinate units co-located with a
// principal that is part of the same relation, so a subordinate deployed
// against two principals legitimately has no relation-unit row for the relation
// belonging to the other principal. Without this skip, the common case of one
// subordinate charm serving several principals would fail validation and block
// the migration.
//
// This mirrors 3.6's state.RelationUnit.Valid, which the 3.6 source precheck
// consulted before requiring a unit to be in scope.
func unitExpectedInRelationScope(
	endpoint modelmigrationinternal.RelationValidationEndpoint,
	relationApps, subordinateApps set.Strings,
	principalApp string,
) bool {
	if !endpoint.ContainerScoped {
		// A globally scoped relation holds every unit of its applications.
		return true
	}
	if principalApp == "" {
		// A principal unit is a member of every relation of its application.
		return true
	}
	// When the rest of the relation is subordinate too there is no principal to
	// be co-located with, so every unit is expected in scope.
	otherApps := relationApps.Difference(set.NewStrings(endpoint.ApplicationName))
	if otherApps.IsEmpty() || otherApps.Difference(subordinateApps).IsEmpty() {
		return true
	}
	return relationApps.Contains(principalApp)
}

// validateSecretBackendsExist fails if any secret backend referenced by the
// model's external secret value refs is unknown to this controller.
func (s *Service) validateSecretBackendsExist(ctx context.Context) error {
	inUse, err := s.modelState.GetSecretBackendUUIDsInUse(ctx)
	if err != nil {
		return errors.Errorf("reading secret backends in use by imported model: %w", err)
	}
	if len(inUse) == 0 {
		return nil
	}

	known, err := s.controllerState.GetKnownSecretBackends(ctx, inUse)
	if err != nil {
		return errors.Errorf("checking secret backends exist for imported model: %w", err)
	}
	knownSet := set.NewStrings(known...)

	unknown := set.NewStrings()
	for _, backend := range inUse {
		if !knownSet.Contains(backend) {
			unknown.Add(backend)
		}
	}
	if !unknown.IsEmpty() {
		return errors.Errorf(
			"imported model references secret backend(s) %q that do not exist on this controller",
			unknown.SortedValues(),
		)
	}
	return nil
}

// validateSecretBackendReferences fails if any externally backed secret
// revision is missing its controller secret_backend_reference row, or the row
// points at a different backend than the revision's value ref.
func (s *Service) validateSecretBackendReferences(ctx context.Context) error {
	modelRefs, err := s.modelState.GetExternalSecretRevisionBackends(ctx)
	if err != nil {
		return errors.Errorf("reading external secret revisions of imported model: %w", err)
	}
	if len(modelRefs) == 0 {
		return nil
	}

	controllerRefs, err := s.controllerState.GetSecretBackendReferencesForModel(ctx, s.modelUUID)
	if err != nil {
		return errors.Errorf("reading secret backend references for imported model: %w", err)
	}

	var missing, mismatched []string
	for revisionUUID, valueRefBackend := range modelRefs {
		referenceBackend, ok := controllerRefs[revisionUUID]
		if !ok {
			missing = append(missing, revisionUUID)
			continue
		}
		if referenceBackend != valueRefBackend {
			mismatched = append(mismatched, revisionUUID)
		}
	}
	sort.Strings(missing)
	sort.Strings(mismatched)

	if len(missing) > 0 {
		return errors.Errorf(
			"imported model is missing secret backend references for %d external secret revision(s): %q",
			len(missing), missing,
		)
	}
	if len(mismatched) > 0 {
		return errors.Errorf(
			"imported model has secret backend references that do not match the secret value refs for %d revision(s): %q",
			len(mismatched), mismatched,
		)
	}
	return nil
}

// MissingAgentBinaryArchitectures returns the architectures for which the
// running machine and unit agents would have no agent binary at the given
// version, checking the controller and model object stores. It returns an empty
// slice when every running architecture has a binary, or for a CAAS model
// (whose agents run from OCI images, not the agent binary store).
//
// The activation path uses it to decide whether the migrated model's agent
// version can safely be bumped to the controller target: 3.6 never changed a
// migrated model's agent version, so a missing binary is never fatal here — the
// caller simply leaves the model at its current version and warns.
func (s *Service) MissingAgentBinaryArchitectures(ctx context.Context, version string) ([]string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	modelType, err := s.modelState.GetModelType(ctx)
	if err != nil {
		return nil, errors.Errorf("getting model type: %w", err)
	}
	if coremodel.ModelType(modelType) == coremodel.CAAS {
		return nil, nil
	}

	required, err := s.modelState.GetRunningAgentArchitectures(ctx)
	if err != nil {
		return nil, errors.Errorf("getting running agent architectures: %w", err)
	}
	if len(required) == 0 {
		return nil, nil
	}

	controllerArchs, err := s.controllerState.GetAgentBinaryArchitecturesForVersion(ctx, version)
	if err != nil {
		return nil, errors.Errorf("getting controller agent binary architectures: %w", err)
	}
	modelArchs, err := s.modelState.GetAgentBinaryArchitecturesForVersion(ctx, version)
	if err != nil {
		return nil, errors.Errorf("getting model agent binary architectures: %w", err)
	}
	available := set.NewStrings(controllerArchs...).Union(set.NewStrings(modelArchs...))

	missing := set.NewStrings()
	for _, arch := range required {
		if !available.Contains(arch) {
			missing.Add(arch)
		}
	}
	return missing.SortedValues(), nil
}
