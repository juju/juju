// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"reflect"

	"github.com/juju/description/v5"
	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/credential"
	"github.com/juju/juju/domain/credential/service"
	"github.com/juju/juju/domain/credential/state"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	// Add adds the given operation to the migration.
	Add(modelmigration.Operation)
}

// RegisterImport registers the import operations with the given coordinator.
func RegisterImport(coordinator Coordinator) {
	coordinator.Add(&importOperation{})
}

// ImportService provides a subset of the credential domain
// service methods needed for credential import.
type ImportService interface {
	CloudCredential(ctx context.Context, id credential.ID) (cloud.Credential, error)
	UpdateCloudCredential(context.Context, credential.ID, cloud.Credential) error
}

type importOperation struct {
	modelmigration.BaseOperation

	service ImportService
}

// Setup implements Operation.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	// We must not use a watcher during migration, so it's safe to pass a
	// nil watcher factory.
	i.service = service.NewService(
		state.NewState(scope.ControllerDB()), nil, logger)
	return nil
}

// Execute the import on the credentials contained in the model.
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	cred := model.CloudCredential()
	if cred == nil {
		return nil
	}

	// Need to add credential or make sure an existing credential
	// matches.
	id := credential.ID{
		Cloud: cred.Cloud(),
		Owner: cred.Owner(),
		Name:  cred.Name(),
	}
	if err := id.Validate(); err != nil {
		return errors.Trace(err)
	}

	existing, err := i.service.CloudCredential(ctx, id)

	if errors.Is(err, errors.NotFound) {
		credential := cloud.NewCredential(
			cloud.AuthType(cred.AuthType()),
			cred.Attributes())
		return errors.Trace(i.service.UpdateCloudCredential(ctx, id, credential))
	} else if err != nil {
		return errors.Trace(err)
	}
	// ensure existing cred matches.
	if existing.AuthType() != cloud.AuthType(cred.AuthType()) {
		return errors.Errorf("credential auth type mismatch: %q != %q", existing.AuthType(), cred.AuthType())
	}
	if !reflect.DeepEqual(existing.Attributes(), cred.Attributes()) {
		return errors.Errorf("credential attribute mismatch: %v != %v", existing.Attributes(), cred.Attributes())
	}
	if existing.Revoked {
		return errors.Errorf("credential %q is revoked", id)
	}
	return nil
}
