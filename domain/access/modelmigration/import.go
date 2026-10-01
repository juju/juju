// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"maps"
	"slices"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/description/v12"

	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	corepermission "github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/access"
	accesserrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/domain/access/internal"
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

// RegisterExternalUsersImport registers the external users import operation
// with the given coordinator. It must be registered before credential import
// since external users may be model owners referenced by credentials. It must
// also run before offer access import, since offer ACLs may name external
// users that are not model members.
func RegisterExternalUsersImport(coordinator Coordinator, clock clock.Clock, logger logger.Logger) {
	coordinator.Add(&importExternalUsersOperation{
		clock:  clock,
		logger: logger,
	})
}

// RegisterImport registers the import operations with the given coordinator.
func RegisterImport(coordinator Coordinator, clock clock.Clock, logger logger.Logger) {
	coordinator.Add(&importOperation{
		clock:  clock,
		logger: logger,
	})
}

// ImportExternalUsersService provides a subset of the access domain
// service methods needed for external user creation during model import.
type ImportExternalUsersService interface {
	// ImportExternalUsers creates external users from a migrated model on the
	// target controller. Permission granting is handled separately.
	ImportExternalUsers(ctx context.Context, users []internal.ExternalUserImport) error
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

type importExternalUsersOperation struct {
	modelmigration.BaseOperation

	service ImportExternalUsersService

	clock  clock.Clock
	logger logger.Logger
}

// Name returns the name of this operation.
func (i *importExternalUsersOperation) Name() string {
	return "import external users"
}

// Setup implements Operation.
func (i *importExternalUsersOperation) Setup(scope modelmigration.Scope) error {
	i.service = service.NewService(state.NewState(scope.ControllerDB(), i.clock, i.logger), i.clock)
	return nil
}

// Execute creates any external users referenced in the model that do not yet
// exist on the target controller. This must run before credential import since
// an external user may be the model owner. They are collected from the model
// users and offer ACLs, whose grants are keyed by user name without validating
// the grantee exists.
func (i *importExternalUsersOperation) Execute(ctx context.Context, model description.Model) error {
	var externalUsers []internal.ExternalUserImport
	seen := set.NewStrings()
	for _, u := range model.Users() {
		name, err := user.NewName(u.Name())
		if err != nil {
			return errors.Errorf("parsing user name %q: %w", u.Name(), err)
		}
		if name.IsLocal() {
			continue
		}
		seen.Add(name.Name())
		externalUsers = append(externalUsers, internal.ExternalUserImport{
			Name:        name,
			DisplayName: u.DisplayName(),
			DateCreated: u.DateCreated(),
		})
	}
	for _, app := range model.Applications() {
		for _, offer := range app.Offers() {
			externalUsers = i.collectOfferACLUsers(offer, seen, externalUsers)
		}
	}
	if len(externalUsers) == 0 {
		return nil
	}
	return i.service.ImportExternalUsers(ctx, externalUsers)
}

// collectOfferACLUsers appends any external user named in the offer ACL that
// has not already been collected from the model users. Such users have no
// display name or creation date in the model description, so they are
// materialised like external users created on first authentication: the user
// name as display name and now as the creation date. Local users and
// everyone@external are skipped since they must already exist on the target
// controller.
//
// seen is deliberately shared across calls and mutated by this function, so
// that a user named in the ACLs of several offers is collected once.
func (i *importExternalUsersOperation) collectOfferACLUsers(
	offer description.ApplicationOffer,
	seen set.Strings,
	externalUsers []internal.ExternalUserImport,
) []internal.ExternalUserImport {
	for _, aclName := range slices.Sorted(maps.Keys(offer.ACL())) {
		name, err := user.NewName(aclName)
		if err != nil {
			// Invalid ACL user names are rejected by the offer access
			// import operation, nothing to create here.
			continue
		}
		if name.IsLocal() || seen.Contains(name.Name()) ||
			name.Name() == corepermission.EveryoneUserName.Name() {
			continue
		}
		seen.Add(name.Name())
		externalUsers = append(externalUsers, internal.ExternalUserImport{
			Name:        name,
			DisplayName: name.Name(),
			DateCreated: i.clock.Now().UTC(),
		})
	}
	return externalUsers
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

// userImport holds the common data needed to create permissions and record
// the last model login for any user (local or external) during import.
type userImport struct {
	Name           user.Name
	Access         corepermission.Access
	LastConnection time.Time
}

func (i *importOperation) collectUsers(
	users []description.User,
) ([]userImport, error) {
	var allUsers []userImport
	for _, u := range users {
		name, err := user.NewName(u.Name())
		if err != nil {
			return nil, errors.Errorf("importing access for user %q: %w", u.Name(), err)
		}
		access := corepermission.Access(u.Access())
		if err := access.Validate(); err != nil {
			return nil, errors.Errorf("importing access for user %q: %w", name, err)
		}
		allUsers = append(allUsers, userImport{
			Name:           name,
			Access:         access,
			LastConnection: u.LastConnection(),
		})
	}
	return allUsers, nil
}

// Execute the import on the model user permissions contained in the model.
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	modelUUID := model.UUID()
	allUsers, err := i.collectUsers(model.Users())
	if err != nil {
		return errors.Errorf("fetching users: %w", err)
	}

	// Grant model permissions and record last model login for all users in a
	// single pass. Both local and external users must already exist on the
	// target controller at this point (external users are created by the
	// importExternalUsersOperation which runs earlier in the pipeline).
	for _, u := range allUsers {
		_, err = i.service.CreatePermission(ctx, corepermission.UserAccessSpec{
			AccessSpec: corepermission.AccessSpec{
				Target: corepermission.ID{
					ObjectType: corepermission.Model,
					Key:        modelUUID,
				},
				Access: u.Access,
			},
			User: u.Name,
		})
		if err != nil && !errors.Is(err, accesserrors.PermissionAlreadyExists) {
			// If the permission already exists then it must be the model owner
			// who is granted admin access when the model is created.
			return errors.Errorf("creating permission for user %q: %w", u.Name, err)
		}

		if !u.LastConnection.IsZero() {
			if err := i.service.SetLastModelLogin(ctx, u.Name, coremodel.UUID(modelUUID), u.LastConnection); err != nil {
				return errors.Errorf("setting model last login for user %q: %w", u.Name, err)
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
