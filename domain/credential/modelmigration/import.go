// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"reflect"

	"github.com/juju/description/v10"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/credential/service"
	"github.com/juju/juju/domain/credential/state"
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

// ImportService provides a subset of the credential domain
// service methods needed for credential import.
type ImportService interface {
	CloudCredential(context.Context, credential.Key) (cloud.Credential, error)
	UpdateCloudCredential(context.Context, credential.Key, cloud.Credential) error
}

type importOperation struct {
	modelmigration.BaseOperation

	logger  logger.Logger
	service ImportService
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import cloud credential"
}

// Setup implements Operation.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	// We must not use a watcher during migration, so it's safe to pass a
	// nil watcher factory.
	i.service = service.NewService(
		state.NewState(scope.ControllerDB()), i.logger)
	return nil
}

// Execute the import on the credentials contained in the model.
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	cred := model.CloudCredential()
	if cred == nil {
		return nil
	}

	name, err := user.NewName(cred.Owner())
	if err != nil {
		return errors.Capture(err)
	}
	// Need to add credential or make sure an existing credential
	// matches.
	key := credential.Key{
		Cloud: cred.Cloud(),
		Owner: name,
		Name:  cred.Name(),
	}
	if err := key.Validate(); err != nil {
		return errors.Capture(err)
	}

	existing, err := i.service.CloudCredential(ctx, key)

	if errors.Is(err, coreerrors.NotFound) {
		credential := cloud.NewCredential(
			cloud.AuthType(cred.AuthType()),
			cred.Attributes())
		return errors.Capture(i.service.UpdateCloudCredential(ctx, key, credential))
	} else if err != nil {
		return errors.Capture(err)
	}
	// ensure existing cred matches.
	if existing.AuthType() != cloud.AuthType(cred.AuthType()) {
		return errors.Errorf("credential auth type mismatch: %q != %q", existing.AuthType(), cred.AuthType())
	}
	if !reflect.DeepEqual(existing.Attributes(), cred.Attributes()) {
		return errors.Errorf("credential attribute mismatch: %v != %v", existing.Attributes(), cred.Attributes())
	}
	if existing.Revoked {
		return errors.Errorf("credential %q is revoked", key)
	}
	return nil
}
