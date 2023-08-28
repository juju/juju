// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v4"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/credential/service"
	"github.com/juju/juju/domain/credential/state"
)

// RegisterExport registers the export operations with the given coordinator.
func RegisterExport(coordinator Coordinator) {
	coordinator.Add(&exportOperation{})
}

// ExportService provides a subset of the credential domain
// service methods needed for credential export.
type ExportService interface {
	CloudCredential(ctx context.Context, tag names.CloudCredentialTag) (cloud.Credential, error)
}

// exportOperation describes a way to execute a migration for
// exporting external controllers.
type exportOperation struct {
	modelmigration.BaseOperation

	service ExportService
}

// Setup implements Operation.
func (e *exportOperation) Setup(scope modelmigration.Scope) error {
	// We must not use a watcher during migration, so it's safe to pass a
	// nil watcher factory.
	e.service = service.NewService(
		state.NewState(scope.ControllerDB()), nil)
	return nil
}

// Execute the export, adding the credentials to the model.
func (e *exportOperation) Execute(ctx context.Context, model description.Model) error {
	// TODO(wallyworld) - implement once we have a functional model db.
	//if credsTag, credsSet := dbModel.CloudCredentialTag(); credsSet && !cfg.SkipCredentials {
	//}
	var tag names.CloudCredentialTag
	cred, err := e.service.CloudCredential(ctx, tag)
	if err != nil {
		return errors.Trace(err)
	}
	model.SetCloudCredential(description.CloudCredentialArgs{
		Owner:      tag.Owner(),
		Cloud:      tag.Cloud(),
		Name:       tag.Name(),
		AuthType:   string(cred.AuthType()),
		Attributes: cred.Attributes(),
	})

	return nil
}
