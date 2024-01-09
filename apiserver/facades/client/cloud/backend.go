// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	stdcontext "context"
	coreuser "github.com/juju/juju/core/user"
	"github.com/juju/names/v5"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/credential"
	credentialservice "github.com/juju/juju/domain/credential/service"
)

// CloudService provides access to clouds.
type CloudService interface {
	ListAll(stdcontext.Context) ([]cloud.Cloud, error)
	Get(stdcontext.Context, string) (*cloud.Cloud, error)
	Save(ctx stdcontext.Context, cld cloud.Cloud) error
	Delete(ctx stdcontext.Context, name string) error
}

// CloudPermissionService provides access to cloud permissions.
type CloudPermissionService interface {
	GetCloudAccess(cloud string, user names.UserTag) (permission.Access, error)
	GetCloudUsers(cloud string) (map[string]permission.Access, error)
	CreateCloudAccess(usr coreuser.User, cloud string, user names.UserTag, access permission.Access) error
	UpdateCloudAccess(cloud string, user names.UserTag, access permission.Access) error
	RemoveCloudAccess(cloud string, user names.UserTag) error
	CloudsForUser(user names.UserTag) ([]cloud.CloudAccess, error)
}

// UserService provides access to users.
type UserService interface {
	GetUserByName(ctx stdcontext.Context, name string) (coreuser.User, error)
}

// ModelCredentialService provides access to model credential info.
type ModelCredentialService interface {
	CredentialModelsAndOwnerAccess(usr coreuser.User, tag names.CloudCredentialTag) ([]cloud.CredentialOwnerModelAccess, error)
}

// CredentialService provides access to the credential domain service.
type CredentialService interface {
	CloudCredential(ctx stdcontext.Context, id credential.ID) (cloud.Credential, error)
	AllCloudCredentialsForOwner(ctx stdcontext.Context, owner string) (map[credential.ID]cloud.Credential, error)
	CloudCredentialsForOwner(ctx stdcontext.Context, owner, cloudName string) (map[string]cloud.Credential, error)
	UpdateCloudCredential(ctx stdcontext.Context, id credential.ID, cred cloud.Credential) error
	RemoveCloudCredential(ctx stdcontext.Context, id credential.ID) error
	WatchCredential(ctx stdcontext.Context, id credential.ID) (watcher.NotifyWatcher, error)
	CheckAndUpdateCredential(ctx stdcontext.Context, id credential.ID, cred cloud.Credential, force bool) ([]credentialservice.UpdateCredentialModelResult, error)
	CheckAndRevokeCredential(ctx stdcontext.Context, id credential.ID, force bool) error
}
