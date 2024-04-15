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
	// ListAll returns a slice Clouds representing all clouds.
	ListAll(stdcontext.Context) ([]cloud.Cloud, error)
	// Cloud return Cloud data for the requested cloud.
	Cloud(stdcontext.Context, string) (*cloud.Cloud, error)
	// CreateCloud creates a new cloud including setting Admin permission
	// for the owner.
	CreateCloud(ctx stdcontext.Context, ownerName string, cloud cloud.Cloud) error
	// UpdateCloud updates the definition of a current cloud.
	UpdateCloud(ctx stdcontext.Context, cld cloud.Cloud) error
	// DeleteCloud removes a cloud, and any permissions associated with it.
	DeleteCloud(ctx stdcontext.Context, name string) error
}

// CloudAccessService provides access to cloud permissions.
type CloudAccessService interface {
	// ReadUserAccessLevelForTarget returns the access level for the provided
	// subject (user) for the given target (cloud).
	ReadUserAccessLevelForTarget(ctx stdcontext.Context, subject string, target corepermission.ID) (corepermission.Access, error)
	// ReadAllUserAccessForTarget  returns the user access for all users for
	// the given target (cloud).
	ReadAllUserAccessForTarget(ctx stdcontext.Context, target corepermission.ID) ([]corepermission.UserAccess, error)
	// CreatePermission sets the access level for a user on the given cloud.
	CreatePermission(ctx stdcontext.Context, spec corepermission.UserAccessSpec) (corepermission.UserAccess, error)
	// UpdatePermission updates the access level for a user on the given cloud.
	UpdatePermission(ctx stdcontext.Context, args access.UpdatePermissionArgs) error
	// ReadAllAccessForUserAndObjectType returns UserAccess for the given
	// subject (user) for all clouds based on objectType.
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
