// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keymanager

import (
	"context"
	"fmt"
	"net/url"

	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	coressh "github.com/juju/juju/core/ssh"
	"github.com/juju/juju/core/user"
	accesserrors "github.com/juju/juju/domain/access/errors"
	keymanagererrors "github.com/juju/juju/domain/keymanager/errors"
	"github.com/juju/juju/rpc/params"
)

// KeyManagerAPI provides api endpoints for manipulating ssh keys
type KeyManagerAPI struct {
	keyManagerService KeyManagerService
	userService       UserService
	modelID           coremodel.UUID
	controllerUUID    string
	authorizer        facade.Authorizer
	check             BlockChecker
	authedUser        names.UserTag
}

func (api *KeyManagerAPI) checkCanRead(ctx context.Context) error {
	if err := api.checkCanWrite(ctx); err == nil {
		return nil
	} else if err != apiservererrors.ErrPerm {
		return errors.Trace(err)
	}
	err := api.authorizer.HasPermission(
		ctx,
		permission.ReadAccess,
		names.NewModelTag(api.modelID.String()),
	)
	return err
}

func (api *KeyManagerAPI) checkCanWrite(ctx context.Context) error {
	ok, err := common.HasModelAdmin(
		ctx,
		api.authorizer,
		names.NewControllerTag(api.controllerUUID),
		names.NewModelTag(api.modelID.String()),
	)
	if err != nil {
		return errors.Trace(err)
	}
	if !ok {
		return apiservererrors.ErrPerm
	}
	return nil
}

// ListKeys returns the authorised ssh keys for the specified users.
func (api *KeyManagerAPI) ListKeys(
	ctx context.Context,
	arg params.ListSSHKeys,
) (params.StringsResults, error) {
	// Here be dragons. This facade call has two users we care about. The first
	// is that of the authenticated user to the api and the second is the user
	// passed in via params. This facade was setup in a manner so that
	// eventually Juju could support adding keys for individual users. We have
	// now partially wired this support up in the service and when adding keys
	// we speicfy the user to which the keys should be added for.
	//
	// Currently the Juju client doesn't support adding keys for a specific user
	// but for the user arg in params the client always sets this to "admin".
	// We can't rely on this value at the moment and have to use the
	// authenticated entity.

	if err := api.checkCanRead(ctx); err != nil {
		return params.StringsResults{}, apiservererrors.ServerError(err)
	}
	if len(arg.Entities.Entities) == 0 {
		return params.StringsResults{}, nil
	}

	results := make([]params.StringsResult, 0, len(arg.Entities.Entities))
	for range arg.Entities.Entities {
		authedUserName, err := user.NewName(api.authedUser.Id())
		if err != nil {
			results = append(results, params.StringsResult{
				Error: apiservererrors.ParamsErrorf(
					params.CodeUserInvalidName,
					"invalid user name: %q",
					api.authedUser.Id(),
				),
			})
			continue
		}

		user, err := api.userService.GetUserByName(ctx, authedUserName)
		if errors.Is(err, accesserrors.UserNotFound) {
			// We are only checking for the authenticated user here and not the
			// user that has been passed in by params. This is because the juju
			// client currently only supplies admin.
			results = append(results, params.StringsResult{
				Error: apiservererrors.ParamsErrorf(
					params.CodeUserNotFound,
					"user %q does not exist",
					api.authedUser.Id(),
				),
			})
			continue
		} else if err != nil {
			return params.StringsResults{}, fmt.Errorf(
				"cannot get user for %q: %w",
				api.authedUser.String(), err,
			)
		}

		keys, err := api.keyManagerService.ListPublicKeysForUser(ctx, user.UUID)
		switch {
		case errors.Is(err, accesserrors.UserNotFound):
			results = append(results, params.StringsResult{
				Error: apiservererrors.ParamsErrorf(
					params.CodeUserNotFound,
					"user %q does not exist",
					api.authedUser.Id(),
				),
			})
			continue
		case err != nil:
			return params.StringsResults{}, fmt.Errorf(
				"cannot get keys for user %q: %w",
				user.Name, err,
			)
		}

		rval := transform.Slice(keys, func(pk coressh.PublicKey) string {
			if arg.Mode == params.SSHListModeFingerprint {
				return pk.Fingerprint
			}
			return pk.Key
		})

		results = append(results, params.StringsResult{
			Result: rval,
		})
	}

	return params.StringsResults{
		Results: results,
	}, nil

}

// AddKeys adds new authorised ssh keys for the specified user.
func (api *KeyManagerAPI) AddKeys(
	ctx context.Context,
	arg params.ModifyUserSSHKeys,
) (params.ErrorResults, error) {
	// Here be dragons. This facade call has two users we care about. THe first
	// is that of the authenticated user to the api and the second is the user
	// passed in via params. This facade was setup in a manner so that
	// eventually Juju could support adding keys for individual users. We have
	// now partially wired this support up in the service and when adding keys
	// we speicfy the user to which the keys should be added for.
	//
	// Currently the Juju client doesn't support adding keys for a specific user
	// but for the user arg in params the client always sets this to "admin".
	// We can't rely on this value at the moment and have to use the
	// authenticated entity.

	if err := api.checkCanWrite(ctx); err != nil {
		return params.ErrorResults{}, apiservererrors.ServerError(err)
	}
	if err := api.check.ChangeAllowed(ctx); err != nil {
		return params.ErrorResults{}, err
	}
	if len(arg.Keys) == 0 {
		return params.ErrorResults{}, nil
	}

	user, err := api.userService.GetUserByName(ctx, user.NameFromTag(api.authedUser))
	switch {
	case errors.Is(err, accesserrors.UserNotFound):
		return params.ErrorResults{
			Results: []params.ErrorResult{
				{
					Error: apiservererrors.ParamsErrorf(
						params.CodeUserNotFound,
						"user %q does not exist",
						arg.User,
					),
				},
			},
		}, nil
	case err != nil:
		return params.ErrorResults{}, fmt.Errorf(
			"cannot get user for entity %q: %w",
			arg.User, err,
		)
	}

	err = api.keyManagerService.AddPublicKeysForUser(ctx, user.UUID, arg.Keys...)
	switch {
	case errors.Is(err, accesserrors.UserNotFound):
		return params.ErrorResults{
			Results: []params.ErrorResult{
				{
					Error: apiservererrors.ParamsErrorf(
						params.CodeUserNotFound,
						"user %q does not exist",
						arg.User,
					),
				},
			},
		}, nil
	case errors.Is(err, keymanagererrors.ReservedCommentViolation):
		return params.ErrorResults{
			Results: []params.ErrorResult{
				{
					Error: apiservererrors.ParamsErrorf(
						params.CodeUserKeyInvalidComment,
						"one or more public keys to be added for user %q contains a restricted comment",
						arg.User,
					),
				},
			},
		}, nil
	case errors.Is(err, keymanagererrors.InvalidPublicKey):
		return params.ErrorResults{
			Results: []params.ErrorResult{
				{
					Error: apiservererrors.ParamsErrorf(
						params.CodeUserKeyInvalidKey,
						"one or more public keys to be added for user %q is invalid",
						arg.User,
					),
				},
			},
		}, nil
	case errors.Is(err, keymanagererrors.PublicKeyAlreadyExists):
		return params.ErrorResults{
			Results: []params.ErrorResult{
				{
					Error: apiservererrors.ParamsErrorf(
						params.CodeUserKeyAlreadyExists,
						"one or more public keys to be added for user %q already exist",
						arg.User,
					),
				},
			},
		}, nil
	case err != nil:
		return params.ErrorResults{}, fmt.Errorf(
			"cannot add keys for user %q: %w", user.Name, err,
		)
	}

	return params.ErrorResults{}, nil
}

// ImportKeys imports new authorised ssh keys from the specified key ids for the
// specified user.
func (api *KeyManagerAPI) ImportKeys(ctx context.Context, arg params.ModifyUserSSHKeys) (params.ErrorResults, error) {
	if err := api.checkCanWrite(ctx); err != nil {
		return params.ErrorResults{}, apiservererrors.ServerError(err)
	}
	if err := api.check.ChangeAllowed(ctx); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	if len(arg.Keys) == 0 {
		return params.ErrorResults{}, nil
	}

	name, err := user.NewName(arg.User)
	if err != nil {
		return params.ErrorResults{
			Results: []params.ErrorResult{
				{
					Error: apiservererrors.ParamsErrorf(
						params.CodeUserInvalidName,
						"invalid user name: %q",
						arg.User,
					),
				},
			},
		}, nil
	}

	user, err := api.userService.GetUserByName(ctx, name)
	switch {
	case errors.Is(err, accesserrors.UserNotFound):
		return params.ErrorResults{
			Results: []params.ErrorResult{
				{
					Error: apiservererrors.ParamsErrorf(
						params.CodeUserNotFound,
						"user %q does not exist",
						arg.User,
					),
				},
			},
		}, nil
	case err != nil:
		return params.ErrorResults{}, fmt.Errorf(
			"cannot get user for entity %q: %w",
			arg.User, err,
		)
	}

	results := make([]params.ErrorResult, 0, len(arg.Keys))
	for _, keySource := range arg.Keys {
		keySourceURL, err := url.Parse(keySource)
		if err != nil {
			results = append(results, params.ErrorResult{
				Error: apiservererrors.ParamsErrorf(
					params.CodeUserKeyInvalidKeySource,
					"parsing key source url %q for public key importing on user %q",
					keySource,
					arg.User,
				),
			})
			continue
		}

		result := params.ErrorResult{}
		err = api.keyManagerService.ImportPublicKeysForUser(ctx, user.UUID, keySourceURL)
		switch {
		case errors.Is(err, accesserrors.UserNotFound):
			result.Error = apiservererrors.ParamsErrorf(
				params.CodeUserNotFound,
				"user %q does not exist",
				arg.User,
			)
		case errors.Is(err, keymanagererrors.InvalidPublicKey):
			result.Error = apiservererrors.ParamsErrorf(
				params.CodeUserKeyInvalidKey,
				"one or more public keys to be imported from %q for user %q is invalid",
				keySource,
				arg.User,
			)
		case errors.Is(err, keymanagererrors.ReservedCommentViolation):
			result.Error = apiservererrors.ParamsErrorf(
				params.CodeUserKeyInvalidComment,
				"one or more public keys to be imported from %q for user %q contains a restricted comment",
				keySource,
				arg.User,
			)
		case errors.Is(err, keymanagererrors.UnknownImportSource):
			result.Error = apiservererrors.ParamsErrorf(
				params.CodeUserKeyUnknownKeySource,
				"cannot import from key source %q for user %q, unknown source",
				keySource,
				arg.User,
			)
		case errors.Is(err, keymanagererrors.ImportSubjectNotFound):
			result.Error = apiservererrors.ParamsErrorf(
				params.CodeUserKeySourceSubjectNotFound,
				"cannot import from key source %q for user %q, subject not found",
				keySource,
				arg.User,
			)
		case err != nil:
			return params.ErrorResults{}, fmt.Errorf(
				"cannot import keys from source %q for user %q: %w",
				keySource,
				user.UUID,
				err,
			)
		}

		results = append(results, result)
	}

	return params.ErrorResults{Results: results}, nil
}

// DeleteKeys deletes the authorised ssh keys for the specified user.
func (api *KeyManagerAPI) DeleteKeys(ctx context.Context, arg params.ModifyUserSSHKeys) (params.ErrorResults, error) {
	if err := api.checkCanWrite(ctx); err != nil {
		return params.ErrorResults{}, apiservererrors.ServerError(err)
	}
	if err := api.check.RemoveAllowed(ctx); err != nil {
		return params.ErrorResults{}, err
	}

	if len(arg.Keys) == 0 {
		return params.ErrorResults{}, nil
	}

	user, err := api.userService.GetUserByName(ctx, user.NameFromTag(api.authedUser))
	switch {
	case errors.Is(err, accesserrors.UserNotFound):
		return params.ErrorResults{
			Results: []params.ErrorResult{
				{
					Error: apiservererrors.ParamsErrorf(
						params.CodeUserNotFound,
						"user %q does not exist",
						arg.User,
					),
				},
			},
		}, nil
	case err != nil:
		return params.ErrorResults{}, fmt.Errorf(
			"cannot get user for entity %q: %w",
			arg.User, err,
		)
	}

	err = api.keyManagerService.DeleteKeysForUser(ctx, user.UUID, arg.Keys...)
	switch {
	case errors.Is(err, accesserrors.UserNotFound):
		return params.ErrorResults{
			Results: []params.ErrorResult{
				{
					Error: apiservererrors.ParamsErrorf(
						params.CodeUserNotFound,
						"user %q does not exist",
						arg.User,
					),
				},
			},
		}, nil
	case err != nil:
		return params.ErrorResults{}, fmt.Errorf(
			"cannot delete %d keys for user %q: %w",
			len(arg.Keys),
			user.UUID,
			err,
		)
	}

	return params.ErrorResults{}, nil
}
