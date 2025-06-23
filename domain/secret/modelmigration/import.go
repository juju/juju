// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"reflect"
	"sort"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/description/v10"
	"github.com/juju/names/v6"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/secrets"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	"github.com/juju/juju/domain/secret/service"
	"github.com/juju/juju/domain/secret/state"
	backendservice "github.com/juju/juju/domain/secretbackend/service"
	secretbackendstate "github.com/juju/juju/domain/secretbackend/state"
	"github.com/juju/juju/internal/errors"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	// Add adds the given operation to the migration.
	Add(modelmigration.Operation)
}

// RegisterImport registers the import operations with the given coordinator.
func RegisterImport(coordinator Coordinator, logger logger.Logger) {
	coordinator.Add(&importOperation{
		logger: logger,
	})
}

// ImportService provides a subset of the secret domain
// service methods needed for secret import.
type ImportService interface {
	ImportSecrets(context.Context, *service.SecretExport) error
}

// SecretBackendService provides a subset of the secret backend
// domain service methods needed for secret import.
type SecretBackendService interface {
	ListBackendIDs(ctx context.Context) ([]string, error)
}

type importOperation struct {
	modelmigration.BaseOperation

	service        ImportService
	backendService SecretBackendService
	logger         logger.Logger

	knownSecretBackends set.Strings
	seenBackendIds      set.Strings
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import secrets"
}

func (i *importOperation) Setup(scope modelmigration.Scope) error {
	// We must not use a watcher during migration, so it's safe to pass a
	// nil watcher factory.
	backendstate := secretbackendstate.NewState(scope.ControllerDB(), i.logger)
	i.service = service.NewSecretService(
		state.NewState(scope.ModelDB(), i.logger),
		backendstate, nil, i.logger,
	)
	i.backendService = backendservice.NewService(
		backendstate, i.logger,
	)
	return nil
}

func ownerFromTag(owner names.Tag) (secrets.Owner, error) {
	switch owner.Kind() {
	case names.ApplicationTagKind:
		return secrets.Owner{Kind: secrets.ApplicationOwner, ID: owner.Id()}, nil
	case names.UnitTagKind:
		return secrets.Owner{Kind: secrets.UnitOwner, ID: owner.Id()}, nil
	case names.ModelTagKind:
		return secrets.Owner{Kind: secrets.ModelOwner, ID: owner.Id()}, nil
	}
	return secrets.Owner{}, errors.Errorf("tag kind %q %w", owner.Kind(), coreerrors.NotValid)
}

func accessorFromTag(tag names.Tag) (service.SecretAccessor, error) {
	result := service.SecretAccessor{
		ID: tag.Id(),
	}
	switch kind := tag.Kind(); kind {
	case names.ApplicationTagKind:
		if strings.HasPrefix(result.ID, "remote-") {
			result.Kind = service.RemoteApplicationAccessor
		} else {
			result.Kind = service.ApplicationAccessor
		}
	case names.UnitTagKind:
		result.Kind = service.UnitAccessor
	case names.ModelTagKind:
		result.Kind = service.ModelAccessor
	default:
		return service.SecretAccessor{}, errors.Errorf("tag kind %q not valid", kind)
	}
	return result, nil
}

func scopeFromTag(scope string) (service.SecretAccessScope, error) {
	tag, err := names.ParseTag(scope)
	if err != nil {
		return service.SecretAccessScope{}, errors.Capture(err)
	}
	result := service.SecretAccessScope{
		ID: tag.Id(),
	}
	switch kind := tag.Kind(); kind {
	case names.ApplicationTagKind:
		result.Kind = service.ApplicationAccessScope
	case names.UnitTagKind:
		result.Kind = service.UnitAccessScope
	case names.RelationTagKind:
		result.Kind = service.RelationAccessScope
	case names.ModelTagKind:
		result.Kind = service.ModelAccessScope
	default:
		return service.SecretAccessScope{}, errors.Errorf("tag kind %q not valid", kind)
	}
	return result, nil
}

// Execute the import on the secrets description.
// It returns an error satisfying [secreterrors.MissingSecretBackendID] if any
// secret refers to a backend ID not on the target controller.
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	backendIDs, err := i.backendService.ListBackendIDs(ctx)
	if err != nil {
		return errors.Errorf("loading secret backend IDs: %w", err)
	}
	i.knownSecretBackends = set.NewStrings(backendIDs...)

	modelSecrets := model.Secrets()
	modelRemoteSecrets := model.RemoteSecrets()
	allSecrets := service.SecretExport{
		Secrets:         make([]*secrets.SecretMetadata, len(modelSecrets)),
		Revisions:       make(map[string][]*secrets.SecretRevisionMetadata),
		Content:         make(map[string]map[int]secrets.SecretData),
		Access:          make(map[string][]service.SecretAccess),
		Consumers:       make(map[string][]service.ConsumerInfo),
		RemoteConsumers: make(map[string][]service.ConsumerInfo),
		RemoteSecrets:   make([]service.RemoteSecret, len(modelRemoteSecrets)),
	}

	i.seenBackendIds = set.NewStrings()
	for j, secret := range modelSecrets {
		ownerTag, err := secret.Owner()
		if err != nil {
			return errors.Errorf("invalid owner for secret %q: %w", secret.Id(), err)
		}
		owner, err := ownerFromTag(ownerTag)
		if err != nil {
			return errors.Errorf("invalid owner for secret %q: %w", secret.Id(), err)
		}
		allSecrets.Secrets[j] = &secrets.SecretMetadata{
			URI:                    &secrets.URI{ID: secret.Id()},
			Version:                secret.Version(),
			Owner:                  owner,
			Description:            secret.Description(),
			Label:                  secret.Label(),
			LatestRevisionChecksum: secret.LatestRevisionChecksum(),
			LatestExpireTime:       secret.LatestExpireTime(),
			AutoPrune:              secret.AutoPrune(),
			CreateTime:             secret.Created(),
			UpdateTime:             secret.Updated(),
		}
		if secret.RotatePolicy() != "" {
			allSecrets.Secrets[j].RotatePolicy = secrets.RotatePolicy(secret.RotatePolicy())
		}

		if secret.NextRotateTime() != nil {
			nextRotateTime := secret.NextRotateTime()
			allSecrets.Secrets[j].NextRotateTime = nextRotateTime
		}

		secretRevisions, secretContent, err := i.collateRevisionInfo(secret.Revisions())
		if err != nil {
			return errors.Errorf("collating revisions for secret %q: %w", secret.Id(), err)
		}
		allSecrets.Revisions[secret.Id()] = secretRevisions
		allSecrets.Content[secret.Id()] = secretContent

		secretAccess, err := i.collateAccess(secret.ACL())
		if err != nil {
			return errors.Errorf("collating access for secret %q: %w", secret.Id(), err)
		}
		allSecrets.Access[secret.Id()] = secretAccess

		secretConsumers, err := i.collateConsumers(secret.Consumers(), secret.LatestRevision())
		if err != nil {
			return errors.Errorf("collating consumers for secret %q: %w", secret.Id(), err)
		}
		allSecrets.Consumers[secret.Id()] = secretConsumers

		remoteConsumers, err := i.collateRemoteConsumers(secret.RemoteConsumers())
		if err != nil {
			return errors.Errorf("collating remote consumers for secret %q: %w", secret.Id(), err)
		}
		allSecrets.RemoteConsumers[secret.Id()] = remoteConsumers
	}

	for j, secret := range modelRemoteSecrets {
		consumer, err := secret.Consumer()
		if err != nil {
			return errors.Errorf("invalid remote secret consumer: %w", err)
		}
		accessor, err := accessorFromTag(consumer)
		if err != nil {
			return errors.Errorf("invalid remote secret consumer: %w", err)
		}
		allSecrets.RemoteSecrets[j] = service.RemoteSecret{
			URI:             &secrets.URI{ID: secret.ID(), SourceUUID: secret.SourceUUID()},
			Label:           secret.Label(),
			CurrentRevision: secret.CurrentRevision(),
			LatestRevision:  secret.LatestRevision(),
			Accessor:        accessor,
		}
	}

	err = i.service.ImportSecrets(ctx, &allSecrets)
	if err != nil {
		return errors.Errorf("cannot import secrets: %w", err)
	}
	return nil
}

func (i *importOperation) collateRevisionInfo(revisions []description.SecretRevision) ([]*secrets.SecretRevisionMetadata, map[int]secrets.SecretData, error) {
	secretRevisions := make([]*secrets.SecretRevisionMetadata, len(revisions))
	secretContent := make(map[int]secrets.SecretData)
	for j, rev := range revisions {
		dataCopy := make(secrets.SecretData)
		for k, v := range rev.Content() {
			dataCopy[k] = v
		}
		var valueRef *secrets.ValueRef
		if len(dataCopy) == 0 {
			// This should ever happen, but just in case, avoid a nil pointer dereference.
			switch v := reflect.ValueOf(rev.ValueRef()); v.Kind() {
			case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
				if v.IsNil() {
					return nil, nil, errors.Errorf("missing content for secret revision %d", rev.Number())
				}
			}
			valueRef = &secrets.ValueRef{
				BackendID:  rev.ValueRef().BackendID(),
				RevisionID: rev.ValueRef().RevisionID(),
			}
			if !secrets.IsInternalSecretBackendID(valueRef.BackendID) && !i.seenBackendIds.Contains(valueRef.BackendID) {
				if !i.knownSecretBackends.Contains(valueRef.BackendID) {
					return nil, nil, errors.Errorf(
						"target controller does not have all required secret backends set up, missing %q",
						valueRef.BackendID).Add(secreterrors.MissingSecretBackendID)

				}
			}
			i.seenBackendIds.Add(valueRef.BackendID)
		} else {
			secretContent[rev.Number()] = dataCopy
		}
		secretRevisions[j] = &secrets.SecretRevisionMetadata{
			Revision:   rev.Number(),
			CreateTime: rev.Created(),
			UpdateTime: rev.Updated(),
			ExpireTime: rev.ExpireTime(),
			ValueRef:   valueRef,
		}
	}
	return secretRevisions, secretContent, nil
}

func (i *importOperation) collateConsumers(consumers []description.SecretConsumer, latestRevision int) ([]service.ConsumerInfo, error) {
	result := make([]service.ConsumerInfo, len(consumers))
	for i, info := range consumers {
		consumer, err := info.Consumer()
		if err != nil {
			return nil, errors.Errorf("invalid consumer: %w", err)
		}
		accessor, err := accessorFromTag(consumer)
		if err != nil {
			return nil, errors.Errorf("invalid consumer: %w", err)
		}
		currentRev := info.CurrentRevision()
		// Older models may have set the consumed rev info to 0 (assuming the latest revision always).
		// So set the latest values explicitly.
		if currentRev == 0 {
			currentRev = latestRevision
		}
		result[i] = service.ConsumerInfo{
			Accessor: accessor,
			SecretConsumerMetadata: secrets.SecretConsumerMetadata{
				Label:           info.Label(),
				CurrentRevision: currentRev,
			},
		}
	}
	return result, nil
}

func (i *importOperation) collateRemoteConsumers(remoteConsumers []description.SecretRemoteConsumer) ([]service.ConsumerInfo, error) {
	result := make([]service.ConsumerInfo, len(remoteConsumers))
	for i, info := range remoteConsumers {
		consumer, err := info.Consumer()
		if err != nil {
			return nil, errors.Errorf("invalid remote consumer: %w", err)
		}
		accessor, err := accessorFromTag(consumer)
		if err != nil {
			return nil, errors.Errorf("invalid remote consumer: %w", err)
		}
		result[i] = service.ConsumerInfo{
			Accessor: accessor,
			SecretConsumerMetadata: secrets.SecretConsumerMetadata{
				CurrentRevision: info.CurrentRevision(),
			},
		}
	}
	return result, nil
}

func (i *importOperation) collateAccess(secretAccess map[string]description.SecretAccess) ([]service.SecretAccess, error) {
	// Sort for testing.
	var consumers []string
	for consumer := range secretAccess {
		consumers = append(consumers, consumer)
	}
	sort.Strings(consumers)

	var result []service.SecretAccess
	for _, consumer := range consumers {
		access := secretAccess[consumer]
		consumerTag, err := names.ParseTag(consumer)
		if err != nil {
			return nil, errors.Errorf("invalid consumer: %w", err)
		}
		accessor, err := accessorFromTag(consumerTag)
		if err != nil {
			return nil, errors.Errorf("invalid consumer: %w", err)
		}
		scope, err := scopeFromTag(access.Scope())
		if err != nil {
			return nil, errors.Errorf("invalid access scope: %w", err)
		}

		result = append(result, service.SecretAccess{
			Scope:   scope,
			Subject: accessor,
			Role:    secrets.SecretRole(access.Role()),
		})
	}
	return result, nil
}
