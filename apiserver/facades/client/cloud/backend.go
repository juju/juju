// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	stdcontext "context"

	"github.com/juju/names/v5"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/watcher"
	credentialservice "github.com/juju/juju/domain/credential/service"
	"github.com/juju/juju/state"
)

// CloudService provides access to clouds.
type CloudService interface {
	ListAll(stdcontext.Context) ([]cloud.Cloud, error)
	Cloud(stdcontext.Context, string) (*cloud.Cloud, error)
	UpsertCloud(ctx stdcontext.Context, cld cloud.Cloud) error
	DeleteCloud(ctx stdcontext.Context, name string) error
}

// CloudPermissionService provides access to cloud permissions.
type CloudPermissionService interface {
	GetCloudAccess(cloud string, user names.UserTag) (permission.Access, error)
	GetCloudUsers(cloud string) (map[string]permission.Access, error)
	CreateCloudAccess(cloud string, user names.UserTag, access permission.Access) error
	UpdateCloudAccess(cloud string, user names.UserTag, access permission.Access) error
	RemoveCloudAccess(cloud string, user names.UserTag) error
	CloudsForUser(user names.UserTag) ([]cloud.CloudAccess, error)
}

// UserService provides access to users.
type UserService interface {
	User(tag names.UserTag) (User, error)
}

// ModelCredentialService provides access to model credential info.
type ModelCredentialService interface {
	CredentialModelsAndOwnerAccess(tag names.CloudCredentialTag) ([]cloud.CredentialOwnerModelAccess, error)
}

// CredentialService provides access to the credential domain service.
type CredentialService interface {
	CloudCredential(ctx stdcontext.Context, key credential.Key) (cloud.Credential, error)
	AllCloudCredentialsForOwner(ctx stdcontext.Context, owner string) (map[credential.Key]cloud.Credential, error)
	CloudCredentialsForOwner(ctx stdcontext.Context, owner, cloudName string) (map[string]cloud.Credential, error)
	UpdateCloudCredential(ctx stdcontext.Context, key credential.Key, cred cloud.Credential) error
	RemoveCloudCredential(ctx stdcontext.Context, key credential.Key) error
	WatchCredential(ctx stdcontext.Context, key credential.Key) (watcher.NotifyWatcher, error)
	CheckAndUpdateCredential(ctx stdcontext.Context, key credential.Key, cred cloud.Credential, force bool) ([]credentialservice.UpdateCredentialModelResult, error)
	CheckAndRevokeCredential(ctx stdcontext.Context, key credential.Key, force bool) error
}

type User interface {
	DisplayName() string
}

type stateShim struct {
	*state.State
}

func (s stateShim) User(tag names.UserTag) (User, error) {
	return s.State.User(tag)
}
