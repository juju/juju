// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v5"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/credential"
	"github.com/juju/juju/domain/credential/service"
	"github.com/juju/juju/domain/credential/state"
)

var logger = loggo.GetLogger("juju.migration.credentials")

// RegisterExport registers the export operations with the given coordinator.
func RegisterExport(coordinator Coordinator) {
	coordinator.Add(&exportOperation{})
}

// ExportService provides a subset of the credential domain
// service methods needed for credential export.
type ExportService interface {
	CloudCredential(ctx context.Context, id credential.ID) (cloud.Credential, error)
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
		state.NewState(scope.ControllerDB()), logger)
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
	id := credential.ID{
		Cloud: credInfo.Cloud(),
		Owner: credInfo.Owner(),
		Name:  credInfo.Name(),
	}
	cred, err := e.service.CloudCredential(ctx, id)
	if err != nil {
		return errors.Trace(err)
	}
	model.SetCloudCredential(description.CloudCredentialArgs{
		Owner:      names.NewUserTag(id.Owner),
		Cloud:      names.NewCloudTag(id.Cloud),
		Name:       id.Name,
		AuthType:   string(cred.AuthType()),
		Attributes: cred.Attributes(),
	})

	return nil
}
