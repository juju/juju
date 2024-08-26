// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"strings"

	"github.com/juju/description/v8"

	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/user"
	accesserrors "github.com/juju/juju/domain/access/errors"
	accessservice "github.com/juju/juju/domain/access/service"
	accessstate "github.com/juju/juju/domain/access/state"
	"github.com/juju/juju/domain/controller"
	"github.com/juju/juju/domain/keymanager/service"
	"github.com/juju/juju/domain/keymanager/state"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/ssh"
)

const (
	// modelConfigKeyAuthorizedKeys is the old model config key that was used
	// to describe authorized keys for a model in model config. This key has
	// been removed since and now resides here for backwards compatibility with
	// 3.x controllers.
	modelConfigKeyAuthorizeKeys = "authorized-keys"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	// Add adds the given operation to the migration.
	Add(modelmigration.Operation)
}

// ImportService represents the service methods needed for importing the
// authorized keys of a model during migration.
type ImportService interface {
	// AddPublicKeysForUser is responsible for adding public keys for a user to a
	// model. The following errors can be expected:
	// - [errors.NotValid] when the user id is not valid
	// - [github.com/juju/juju/domain/access/errors.UserNotFound] when the user does
	// not exist.
	// - [keyserrors.InvalidPublicKey] when a public key fails validation.
	// - [keyserrors.ReservedCommentViolation] when a key being added contains a
	// comment string that is reserved.
	// - [keyserrors.PublicKeyAlreadyExists] when a public key being added
	// for a user already exists.
	AddPublicKeysForUser(context.Context, user.UUID, ...string) error
}

// UserService represents the service methods needed for finding users when
// importing ssh public keys for a model.
type UserService interface {
	// GetUserByName will find and return the user associated with name. If
	// there is no user for the user name then an error that satisfies
	// [github.com/juju/juju/domain/access/errors].NotFound will be returned.
	// If supplied with an invalid user name then an error that satisfies
	// [github.com/juju/juju/domain/access/errors].UserNameNotValid will be
	// returned.
	GetUserByName(context.Context, user.Name) (user.User, error)

	// GetUsernamesForIds is responsible for returning all of the [user.Name]
	// for each of the [user.UUID] passed to this function. Should one of the
	// user id's not exist then the whole call fails with a
	// [github.com/juju/juju/domain/access/errors].UserNotFound error returned.
	GetUsernamesForIds(context.Context, ...user.UUID) (map[user.UUID]user.Name, error)
}

// importOperation is the type used to describe the import operation for
// authorized keys between models.
type importOperation struct {
	modelmigration.BaseOperation

	// service is the [ImportService] to use during this operation.
	service     ImportService
	userService UserService
}

// Execute the import of the model description authorized keys.
func (i *importOperation) Execute(
	ctx context.Context,
	model description.Model,
) error {
	if err := i.executeModelConfigAuthorizedKeys(ctx, model); err != nil {
		return err
	}

	// After attempting to import older keys located in model config we can try
	// and import by the new method off of description.
	usersAuthorizedKeys := model.AuthorizedKeys()
	for _, uak := range usersAuthorizedKeys {
		userName, err := user.NewName(uak.Username())
		if err != nil {
			return errors.Errorf(
				"cannot import authorized keys for user %q on model when constructing user name: %w",
				uak.Username(), err,
			)
		}

		user, err := i.userService.GetUserByName(ctx, userName)
		if errors.Is(err, accesserrors.UserNotFound) {
			return errors.Errorf(
				"cannot import authorized keys for user %q on model, user does not exist in the model",
				userName,
			).Add(err)
		} else if err != nil {
			return errors.Errorf(
				"cannot import authorized keys for user %q on model when finding user: %w",
				userName,
				err,
			)
		}

		err = i.service.AddPublicKeysForUser(ctx, user.UUID, uak.AuthorizedKeys()...)
		if err != nil {
			return errors.Errorf(
				"cannot import authorized keys for user %q on model: %w",
				userName,
				err,
			)
		}
	}

	return nil
}

// executeModelConfigAuthorizedKeys is responsible for importing a models
// authorized keys when they are still contained with the models config. When we
// detect that we are importing a model that has still been storing authorized
// keys within model config we want to pull these keys out and import them into
// the model under the admin user.
//
// NOTE (tlm): It was chosen to perform this logic and action here instead of in
// the description package between description versions because it would have
// required the description package to start getting internal business logic
// about what user owns what key.
func (i *importOperation) executeModelConfigAuthorizedKeys(
	ctx context.Context,
	model description.Model,
) error {
	authKeysI, has := model.Config()[modelConfigKeyAuthorizeKeys]
	if !has {
		// No authorized keys in model config so we can safely just get out of
		// here.
		return nil
	}
	authKeys, isString := authKeysI.(string)
	if !isString {
		return errors.New("cannot import authorized keys from model config, expected a string")
	}

	// Because of bugs over time there are Juju controllers out in the wild that
	// have the controllers ssh key baked into each and every models authorized
	// keys. We can't fix this now as the damage is done.
	//
	// But we can do our best to stop it coming across when importing models. It
	// would be a security bug if an old controller could still access the
	// machines of a model that it does not own anymore.
	publicKeys, err := ssh.SplitAuthorizedKeys(authKeys)
	if err != nil {
		return errors.Errorf(
			"cannot split authorized keys in model config during import: %w",
			err,
		)
	}

	cleansedKeys := make([]string, 0, len(publicKeys))
	for _, pk := range publicKeys {
		if strings.Contains(pk, controller.ControllerSSHKeyComment) {
			continue
		}
		cleansedKeys = append(cleansedKeys, pk)
	}

	adminUser, err := i.userService.GetUserByName(ctx, user.AdminUserName)
	if err != nil {
		return errors.New(
			"cannot import authorized keys from model config. " +
				"Finding admin user to assign owner ship of keys",
		)
	}

	err = i.service.AddPublicKeysForUser(ctx, adminUser.UUID, cleansedKeys...)
	if err != nil {
		return errors.Errorf(
			"cannot add public keys for the admin user from model config: %w",
			err,
		)
	}
	return nil
}

// Name returns the user readable name for this import operation.
func (i *importOperation) Name() string {
	return "import model authorized keys"
}

// RegisterImport register's a new model authorized keys importer into the
// supplied coordinator.
func RegisterImport(coordinator Coordinator) {
	coordinator.Add(&importOperation{})
}

// Setup the import operation, this will ensure the service is created and ready
// to be used.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.service = service.NewService(state.NewState(scope.ModelDB()))
	i.userService = accessservice.NewUserService(accessstate.NewUserState(scope.ControllerDB()))
	return nil
}
