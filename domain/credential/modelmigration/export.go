// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/credential/service"
	"github.com/juju/juju/domain/credential/state"
	"github.com/juju/juju/internal/errors"
)

// RegisterExport registers the export operations with the given coordinator.
func RegisterExport(coordinator Coordinator, logger logger.Logger) {
	coordinator.Add(&exportOperation{
		logger: logger,
	})
}

// ExportService provides a subset of the credential domain
// service methods needed for credential export.
type ExportService interface {
	CloudCredential(ctx context.Context, key credential.Key) (cloud.Credential, error)
}

// exportOperation describes a way to execute a migration for
// exporting external controllers.
type exportOperation struct {
	modelmigration.BaseOperation

	logger  logger.Logger
	service ExportService
}

// Name returns the name of this operation.
func (e *exportOperation) Name() string {
	return "export cloud credential"
}

// Setup implements Operation.
func (e *exportOperation) Setup(scope modelmigration.Scope) error {
	// We must not use a watcher during migration, so it's safe to pass a
	// nil watcher factory.
	e.service = service.NewService(
		state.NewState(scope.ControllerDB()), e.logger)
	return nil
}

// Execute the export, adding the credentials to the model.
func (e *exportOperation) Execute(ctx context.Context, model description.Model) error {
	// TODO(wallyworld) - implement properly once we have a functional model db.
	//if credsTag, credsSet := dbModel.CloudCredentialTag(); credsSet && !cfg.SkipCredentials {
	//}
	credInfo := model.CloudCredential()
	if credInfo == nil || credInfo.Name() == "" {
		// Not set.
		return nil
	}
	name, err := user.NewName(credInfo.Owner())
	if err != nil {
		return errors.Capture(err)
	}
	key := credential.Key{
		Cloud: credInfo.Cloud(),
		Owner: name,
		Name:  credInfo.Name(),
	}
	cred, err := e.service.CloudCredential(ctx, key)
	if err != nil {
		return errors.Capture(err)
	}
	model.SetCloudCredential(description.CloudCredentialArgs{
		Owner:      key.Owner.Name(),
		Cloud:      key.Cloud,
		Name:       key.Name,
		AuthType:   string(cred.AuthType()),
		Attributes: cred.Attributes(),
	})

	return nil
}
