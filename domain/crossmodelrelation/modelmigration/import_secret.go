// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/description/v12"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/crossmodelrelation/service"
	modelstate "github.com/juju/juju/domain/crossmodelrelation/state/model"
	domainmodelmigration "github.com/juju/juju/domain/modelmigration/modelmigration"
	"github.com/juju/juju/internal/errors"
)

// RegisterImportSecret registers the import secret operations with the given coordinator.
func RegisterImportSecret(coordinator Coordinator, clock clock.Clock, logger logger.Logger) {
	coordinator.Add(&importSecretOperation{
		clock:  clock,
		logger: logger,
	})
}

// ImportSecretService provides a subset of the cross model relation domain
// service methods needed for import.
type ImportSecretService interface {
	// ImportGrantedSecrets imports secrets granted by offerer applications to
	// consumer applications in the offerer model.
	ImportGrantedSecrets(ctx context.Context, grantedSecrets []service.GrantedSecretImport) error

	// ImportRemoteSecrets imports secrets granted by offerer applications to
	// consumer applications in the consumer model.
	ImportRemoteSecrets(ctx context.Context, remoteSecrets []service.RemoteSecretImport) error
}

type importSecretOperation struct {
	modelmigration.BaseOperation

	importService ImportSecretService

	clock  clock.Clock
	logger logger.Logger
}

// Name returns the name of this operation.
func (i *importSecretOperation) Name() string {
	return "import remote secrets"
}

// Setup implements Operation.
func (i *importSecretOperation) Setup(scope modelmigration.Scope) error {
	i.importService = service.NewMigrationService(
		modelstate.NewState(scope.ModelDB(), "", i.clock, i.logger),
		i.logger,
	)
	return nil
}

// Execute the import of the remote secrets for both offerer and consumers
func (i *importSecretOperation) Execute(ctx context.Context, model description.Model) error {
	// Extract secret granted to remote applications. Secrets value should already
	// have been imported by the secrets domain, so we just need to know which
	// secrets were granted to remote applications with which ACLs in order to
	// re-create the same grants in the new model.
	remoteGrantedSecrets, err := extractRemoteGrantedSecrets(model)
	if err != nil {
		return errors.Errorf("extracting remote granted secrets: %w", err)
	}

	// Import granted secrets from remote applications.
	if err := i.importService.ImportGrantedSecrets(ctx, remoteGrantedSecrets); err != nil {
		return errors.Errorf("importing remote granted secrets: %w", err)
	}

	// Extract consumed secrets from remote applications.
	remoteSecrets, err := extractRemoteSecrets(model)
	if err != nil {
		return errors.Errorf("extracting remote secrets: %w", err)
	}

	if err := i.importService.ImportRemoteSecrets(ctx, remoteSecrets); err != nil {
		return errors.Errorf("importing remote secrets: %w", err)
	}

	return nil
}

func extractRemoteGrantedSecrets(model description.Model) ([]service.GrantedSecretImport, error) {
	var grantedSecrets []service.GrantedSecretImport
	for _, secret := range model.Secrets() {
		remoteGrants, err := extractRemoteGrants(secret)
		if err != nil {
			return nil, errors.Errorf("extracting remote grants for secret %q: %w", secret.Id(), err)
		}
		consumers, err := extractRemoteConsumers(secret)
		if err != nil {
			return nil, errors.Errorf("extracting remote consumers for secret %q: %w", secret.Id(), err)
		}

		// If there is no remote grant nor consumer, ignore the secret (it is probably a local secret)
		if len(remoteGrants)+len(consumers) == 0 {
			continue
		}

		grantedSecrets = append(grantedSecrets, service.GrantedSecretImport{
			SecretID:  secret.Id(),
			ACLs:      remoteGrants,
			Consumers: consumers,
		})
	}
	return grantedSecrets, nil
}

func extractRemoteGrants(secret description.Secret) ([]service.GrantedSecretACLImport, error) {
	var result []service.GrantedSecretACLImport
	for app, acl := range secret.ACL() {
		// We only care about grants to remote applications,
		// which are identified by the application name being in the format
		// "remote-<uuid>".
		tag, err := names.ParseTag(app)
		if err != nil {
			return nil, errors.Errorf("parsing application tag from secret ACL key %q: %w", app, err)
		}
		if !domainmodelmigration.IsRemoteSecretGrant(tag) {
			// Not a remote application, skip since remote secrets are granted
			// to applications through relations.
			continue
		}

		relKeyTag, err := names.ParseRelationTag(acl.Scope())
		if err != nil {
			return nil, errors.Errorf("parsing relation tag from secret ACL scope %q: %w", acl.Scope(), err)
		}
		relKey, err := relation.NewKeyFromString(relKeyTag.Id())
		if err != nil {
			return nil, errors.Errorf("parsing relation key from secret ACL scope %q: %w", acl.Scope(), err)
		}

		result = append(result, service.GrantedSecretACLImport{
			ApplicationName: tag.Id(),
			RelationKey:     relKey,
			Role:            secrets.SecretRole(acl.Role()),
		})
	}
	return result, nil
}

func extractRemoteConsumers(secret description.Secret) ([]service.GrantedSecretConsumerImport, error) {
	var result []service.GrantedSecretConsumerImport
	for _, consumer := range secret.RemoteConsumers() {
		tag, err := consumer.Consumer()
		if err != nil {
			return nil, errors.Errorf("parsing consumer tag from secret remote consumer: %w", err)
		}
		if tag.Kind() != names.UnitTagKind {
			return nil, errors.Errorf("expected consumer tag to be unit tag, got %q for consumer %q", tag.Kind(), tag.Id())
		}
		result = append(result, service.GrantedSecretConsumerImport{
			Unit:            unit.Name(tag.Id()),
			CurrentRevision: consumer.CurrentRevision(),
		})
	}
	return result, nil
}

func extractRemoteSecrets(model description.Model) ([]service.RemoteSecretImport, error) {
	var result []service.RemoteSecretImport
	for _, secret := range model.RemoteSecrets() {
		consumer, err := secret.Consumer()
		if err != nil {
			return nil, errors.Errorf("parsing consumer tag from remote secret: %w", err)
		}
		if consumer.Kind() != names.UnitTagKind {
			return nil, errors.Errorf("expected consumer tag to be unit tag, got %q for remote secret %q", consumer.Kind(), secret.ID())
		}
		result = append(result, service.RemoteSecretImport{
			SecretID:        secret.ID(),
			SourceUUID:      secret.SourceUUID(),
			Label:           secret.Label(),
			ConsumerUnit:    unit.Name(consumer.Id()),
			CurrentRevision: secret.CurrentRevision(),
			LatestRevision:  secret.LatestRevision(),
		})
	}
	return result, nil
}
