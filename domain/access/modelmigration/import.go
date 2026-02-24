// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/description/v11"

	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	corepermission "github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/access"
	accesserrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/domain/access/service"
	"github.com/juju/juju/domain/access/state"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	// Add adds the given operation to the migration.
	Add(modelmigration.Operation)
}

// RegisterImport registers the import operations with the given coordinator.
func RegisterImport(coordinator Coordinator, clock clock.Clock, logger logger.Logger) {
	coordinator.Add(&importOperation{
		clock:  clock,
		logger: logger,
	})
}

// ImportService provides a subset of the access domain
// service methods needed for model permissions import.
type ImportService interface {
	// CreatePermission gives the user access per the provided spec.
	// If the user provided does not exist or is marked removed,
	// [accesserrors.PermissionNotFound] is returned.
	// If the user provided exists but is marked disabled,
	// [accesserrors.UserAuthenticationDisabled] is returned.
	// If a permission for the user and target key already exists,
	// [accesserrors.PermissionAlreadyExists] is returned.
	CreatePermission(ctx context.Context, spec corepermission.UserAccessSpec) (corepermission.UserAccess, error)
	// SetLastModelLogin will set the last login time for the user to the given
	// value. The following error types are possible from this function:
	// [accesserrors.UserNameNotValid] when the username supplied is not valid.
	// [accesserrors.UserNotFound] when the user cannot be found.
	// [modelerrors.NotFound] if no model by the given modelUUID exists.
	SetLastModelLogin(ctx context.Context, name user.Name, modelUUID coremodel.UUID, time time.Time) error
}

// ImportOfferAccessService provides a subset of the access domain
// service methods needed for offer permissions import.
type ImportOfferAccessService interface {
	// DeletePermissionsByGrantOnUUID remove permissions for given GrantOn
	// UUIDs.
	DeletePermissionsByGrantOnUUID(ctx context.Context, grantOnUUIDs []string) error
	// ImportOfferAccess imports the user access for offers in the
	// model.
	ImportOfferAccess(ctx context.Context, importAccess []access.OfferImportAccess) error
}

type importOperation struct {
	modelmigration.BaseOperation

	service ImportService

	clock  clock.Clock
	logger logger.Logger
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import model user permissions"
}

// Setup implements Operation.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.service = service.NewService(state.NewState(scope.ControllerDB(), i.clock, i.logger), i.clock)
	return nil
}

// Execute the import on the model user permissions contained in the model.
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	modelUUID := model.UUID()
	for _, u := range model.Users() {
		name, err := user.NewName(u.Name())
		if err != nil {
			return errors.Errorf("importing access for user %q: %w", u.Name(), err)
		}
		access := corepermission.Access(u.Access())
		if err := access.Validate(); err != nil {
			return errors.Errorf("importing access for user %q: %w", name, err)
		}
		_, err = i.service.CreatePermission(ctx, corepermission.UserAccessSpec{
			AccessSpec: corepermission.AccessSpec{
				Target: corepermission.ID{
					ObjectType: corepermission.Model,
					Key:        modelUUID,
				},
				Access: access,
			},
			User: name,
		})
		if err != nil && !errors.Is(err, accesserrors.PermissionAlreadyExists) {
			// If the permission already exists then it must be the model owner
			// who is granted admin access when the model is created.
			return errors.Errorf("creating permission for user %q: %w", name, err)
		}

		lastLogin := u.LastConnection()
		if !lastLogin.IsZero() {
			err := i.service.SetLastModelLogin(ctx, name, coremodel.UUID(modelUUID), lastLogin)
			if err != nil {
				return errors.Errorf("setting model last login for user %q: %w", name, err)
			}
		}

	}
	return nil
}

// RegisterOfferAccessImport registers offer access import operations with the
// given coordinator.
func RegisterOfferAccessImport(coordinator Coordinator, clock clock.Clock, logger logger.Logger) {
	coordinator.Add(&offerAccessImportOperation{
		clock:  clock,
		logger: logger,
	})
}

type offerAccessImportOperation struct {
	modelmigration.BaseOperation

	service ImportOfferAccessService

	clock  clock.Clock
	logger logger.Logger
}

// Name returns the name of this operation.
func (i *offerAccessImportOperation) Name() string {
	return "import offer user permissions"
}

// Setup implements Operation.
func (i *offerAccessImportOperation) Setup(scope modelmigration.Scope) error {
	i.service = service.NewService(state.NewState(scope.ControllerDB(), i.clock, i.logger), i.clock)
	return nil
}

// Execute the import on the offer permissions contained in the model.
func (i *offerAccessImportOperation) Execute(ctx context.Context, model description.Model) error {
	input := make([]access.OfferImportAccess, 0)
	apps := model.Applications()

	for _, app := range apps {
		for _, offer := range app.Offers() {
			offerUUID, err := uuid.UUIDFromString(offer.OfferUUID())
			if err != nil {
				return errors.Errorf("uuid for offer %q,%q: %w",
					offer.ApplicationName(), offer.OfferName(), err)
			}

			acl, err := encodeImportACL(offer.ACL())
			if err != nil {
				return errors.Errorf("offer %q: %w", offer.OfferName(), err)
			}

			imp := access.OfferImportAccess{
				UUID:   offerUUID,
				Access: acl,
			}
			input = append(input, imp)
		}
	}
	if len(input) == 0 {
		return nil
	}
	return i.service.ImportOfferAccess(ctx, input)
}

// Rollback removes the offer permissions added to the controller db
// when called. Rollback is important as there is no reference to the
// offers in the controller DB, there is no referential integrity.
func (i *offerAccessImportOperation) Rollback(ctx context.Context, model description.Model) error {
	offerUUIDs := make([]string, 0)

	for _, app := range model.Applications() {
		for _, offer := range app.Offers() {
			offerUUIDs = append(offerUUIDs, offer.OfferUUID())
		}
	}
	if len(offerUUIDs) == 0 {
		return nil
	}
	return i.service.DeletePermissionsByGrantOnUUID(ctx, offerUUIDs)
}

func encodeImportACL(input map[string]string) (map[string]corepermission.Access, error) {
	output := make(map[string]corepermission.Access)
	for name, accessVal := range input {
		access := corepermission.Access(accessVal)
		if err := access.Validate(); err != nil {
			return nil, errors.Errorf("encoding access for user %q: %w", name, err)
		}
		output[name] = access
	}
	return output, nil
}
