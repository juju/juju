// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	stdcontext "context"

	"github.com/juju/names/v5"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	corepermission "github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/access"
	credentialservice "github.com/juju/juju/domain/credential/service"
)

// CloudService provides access to clouds.
type CloudService interface {
	ListAll(stdcontext.Context) ([]cloud.Cloud, error)
	Cloud(stdcontext.Context, string) (*cloud.Cloud, error)
	UpsertCloud(ctx stdcontext.Context, userName string, cld cloud.Cloud) error
	DeleteCloud(ctx stdcontext.Context, name string) error
}

// CloudAccessService provides access to cloud permissions.
type CloudAccessService interface {
	ReadUserAccessLevelForTarget(ctx stdcontext.Context, subject string, target corepermission.ID) (corepermission.Access, error)
	ReadAllUserAccessForTarget(ctx stdcontext.Context, target corepermission.ID) ([]corepermission.UserAccess, error)
	CreatePermission(ctx stdcontext.Context, spec corepermission.UserAccessSpec) (corepermission.UserAccess, error)
	UpdatePermission(ctx stdcontext.Context, args access.UpdatePermissionArgs) error
	ReadAllAccessForUserAndObjectType(ctx stdcontext.Context, subject string, objectType corepermission.ObjectType) ([]corepermission.UserAccess, error)
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
