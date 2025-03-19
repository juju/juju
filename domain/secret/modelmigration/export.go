// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"
	"github.com/juju/names/v6"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/domain/secret/service"
	"github.com/juju/juju/domain/secret/state"
	secretbackendstate "github.com/juju/juju/domain/secretbackend/state"
	"github.com/juju/juju/internal/errors"
)

// RegisterExport registers the export operations with the given coordinator.
func RegisterExport(coordinator Coordinator, logger logger.Logger) {
	coordinator.Add(&exportOperation{
		logger: logger,
	})
}

// ExportService provides a subset of the secret domain
// service methods needed for secret export.
type ExportService interface {
	GetSecretsForExport(ctx context.Context) (*service.SecretExport, error)
}

// exportOperation describes a way to execute a migration for
// exporting external controllers.
type exportOperation struct {
	modelmigration.BaseOperation

	service ExportService
	logger  logger.Logger
}

// Name returns the name of this operation.
func (e *exportOperation) Name() string {
	return "export secrets"
}

func (e *exportOperation) Setup(scope modelmigration.Scope) error {
	e.service = service.NewSecretService(
		state.NewState(scope.ModelDB(), e.logger),
		secretbackendstate.NewState(scope.ControllerDB(), e.logger),
		nil,
		e.logger,
	)
	return nil
}

func ownerTagFromOwner(owner secrets.Owner) (names.Tag, error) {
	switch owner.Kind {
	case secrets.UnitOwner:
		return names.NewUnitTag(owner.ID), nil
	case secrets.ApplicationOwner:
		return names.NewApplicationTag(owner.ID), nil
	case secrets.ModelOwner:
		return names.NewModelTag(owner.ID), nil
	}
	return nil, errors.Errorf("owner kind %q %w", owner.Kind, coreerrors.NotValid)
}

func scopeTagFromAccessScope(scope service.SecretAccessScope) (names.Tag, error) {
	switch scope.Kind {
	case service.ApplicationAccessScope:
		return names.NewApplicationTag(scope.ID), nil
	case service.UnitAccessScope:
		return names.NewUnitTag(scope.ID), nil
	case service.RelationAccessScope:
		return names.NewRelationTag(scope.ID), nil
	case service.ModelAccessScope:
		return names.NewModelTag(scope.ID), nil
	}
	return nil, errors.Errorf("scope kind %q %w", scope.Kind, coreerrors.NotValid)
}

func accessorTagFromAccessor(subject service.SecretAccessor) (names.Tag, error) {
	switch subject.Kind {
	case service.ApplicationAccessor:
		return names.NewApplicationTag(subject.ID), nil
	case service.UnitAccessor:
		return names.NewUnitTag(subject.ID), nil
	case service.ModelAccessor:
		return names.NewModelTag(subject.ID), nil
	case service.RemoteApplicationAccessor:
		return names.NewApplicationTag(subject.ID), nil
	}
	return nil, errors.Errorf("subject kind %q %w", subject.Kind, coreerrors.NotValid)
}

// Execute exports any secrets to the specified model.
func (e *exportOperation) Execute(ctx context.Context, model description.Model) error {
	allSecrets, err := e.service.GetSecretsForExport(ctx)
	if err != nil {
		return errors.Errorf("failed to list secrets for export: %w", err)
	}

	accessArgsByID, err := e.collateAccess(allSecrets.Access)
	if err != nil {
		return errors.Errorf("collating secret access args: %w", err)
	}

	consumersByID, err := e.collateConsumers(allSecrets.Consumers)
	if err != nil {
		return errors.Errorf("collating secret consumer args: %w", err)
	}

	remoteConsumersByID, err := e.collateRemoteConsumers(allSecrets.RemoteConsumers)
	if err != nil {
		return errors.Errorf("collating secret remote consumer args: %w", err)
	}

	for _, md := range allSecrets.Secrets {
		ownerTag, err := ownerTagFromOwner(md.Owner)
		if err != nil {
			// Should never happen.
			return errors.Errorf("invalid secret owner %q: %w", md.Owner.ID, err)
		}

		revisionArgsByID := make(map[string][]description.SecretRevisionArgs)
		for _, rev := range allSecrets.Revisions[md.URI.ID] {
			revArg := description.SecretRevisionArgs{
				Number:     rev.Revision,
				Created:    rev.CreateTime,
				Updated:    rev.UpdateTime,
				ExpireTime: rev.ExpireTime,
			}

			content, ok := allSecrets.Content[md.URI.ID][rev.Revision]
			if ok && len(content) > 0 {
				revArg.Content = content
			} else if rev.ValueRef != nil {
				revArg.ValueRef = &description.SecretValueRefArgs{
					BackendID:  rev.ValueRef.BackendID,
					RevisionID: rev.ValueRef.RevisionID,
				}
			} else {
				return errors.Errorf("missing secret content to export for secret %q", md.URI.ID)
			}
			revisionArgsByID[md.URI.ID] = append(revisionArgsByID[md.URI.ID], revArg)
		}

		arg := description.SecretArgs{
			ID:                     md.URI.ID,
			Version:                md.Version,
			Description:            md.Description,
			Label:                  md.Label,
			RotatePolicy:           md.RotatePolicy.String(),
			AutoPrune:              md.AutoPrune,
			Owner:                  ownerTag,
			Created:                md.CreateTime,
			Updated:                md.UpdateTime,
			NextRotateTime:         md.NextRotateTime,
			LatestRevisionChecksum: md.LatestRevisionChecksum,
			Revisions:              revisionArgsByID[md.URI.ID],
			ACL:                    accessArgsByID[md.URI.ID],
			Consumers:              consumersByID[md.URI.ID],
			RemoteConsumers:        remoteConsumersByID[md.URI.ID],
		}
		model.AddSecret(arg)
	}

	for _, rs := range allSecrets.RemoteSecrets {
		consumerTag, err := accessorTagFromAccessor(rs.Accessor)
		if err != nil {
			// Should never happen.
			return errors.Errorf("invalid remote secret accessor %q: %w", rs.Accessor.ID, err)
		}
		arg := description.RemoteSecretArgs{
			ID:              rs.URI.ID,
			SourceUUID:      rs.URI.SourceUUID,
			Consumer:        consumerTag,
			Label:           rs.Label,
			CurrentRevision: rs.CurrentRevision,
			LatestRevision:  rs.LatestRevision,
		}
		model.AddRemoteSecret(arg)
	}
	return nil
}

func (e *exportOperation) collateConsumers(consumers map[string][]service.ConsumerInfo) (map[string][]description.SecretConsumerArgs, error) {
	consumersByID := make(map[string][]description.SecretConsumerArgs)
	for id, infos := range consumers {
		for _, info := range infos {
			consumerTag, err := accessorTagFromAccessor(info.Accessor)
			if err != nil {
				// Should never happen.
				return nil, errors.Errorf("invalid secret consumer %q: %w", info.Accessor.ID, err)
			}
			consumerArg := description.SecretConsumerArgs{
				Consumer:        consumerTag,
				Label:           info.Label,
				CurrentRevision: info.CurrentRevision,
			}
			consumersByID[id] = append(consumersByID[id], consumerArg)
		}
	}
	return consumersByID, nil
}

func (e *exportOperation) collateRemoteConsumers(consumers map[string][]service.ConsumerInfo) (map[string][]description.SecretRemoteConsumerArgs, error) {
	consumersByID := make(map[string][]description.SecretRemoteConsumerArgs)
	for id, infos := range consumers {
		for _, info := range infos {
			consumerTag, err := accessorTagFromAccessor(info.Accessor)
			if err != nil {
				// Should never happen.
				return nil, errors.Errorf("invalid remote secret consumer %q: %w", info.Accessor.ID, err)
			}
			consumerArg := description.SecretRemoteConsumerArgs{
				Consumer:        consumerTag,
				CurrentRevision: info.CurrentRevision,
			}
			consumersByID[id] = append(consumersByID[id], consumerArg)
		}
	}
	return consumersByID, nil
}

func (e *exportOperation) collateAccess(access map[string][]service.SecretAccess) (map[string]map[string]description.SecretAccessArgs, error) {
	accessArgsByID := make(map[string]map[string]description.SecretAccessArgs)
	for id, perms := range access {
		for _, perm := range perms {
			scopeTag, err := scopeTagFromAccessScope(perm.Scope)
			if err != nil {
				// Should never happen.
				return nil, errors.Errorf("invalid secret access scope %q: %w", perm.Scope.ID, err)
			}
			accessArg := description.SecretAccessArgs{
				Scope: scopeTag.String(),
				Role:  string(perm.Role),
			}
			access, ok := accessArgsByID[id]
			if !ok {
				access = make(map[string]description.SecretAccessArgs)
				accessArgsByID[id] = access
			}
			accessorTag, err := accessorTagFromAccessor(perm.Subject)
			if err != nil {
				// Should never happen.
				return nil, errors.Errorf("invalid secret accessor %q: %w", perm.Subject.ID, err)
			}
			access[accessorTag.String()] = accessArg
		}
	}
	return accessArgsByID, nil
}
