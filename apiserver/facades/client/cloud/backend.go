// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"context"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	corepermission "github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/access"
	credentialservice "github.com/juju/juju/domain/credential/service"
)

// CloudService provides access to clouds.
type CloudService interface {
	// ListAll returns a slice Clouds representing all clouds.
	ListAll(context.Context) ([]cloud.Cloud, error)
	// Cloud return Cloud data for the requested cloud.
	Cloud(context.Context, string) (*cloud.Cloud, error)
	// CreateCloud creates a new cloud including setting Admin permission
	// for the owner.
	CreateCloud(ctx context.Context, ownerName user.Name, cloud cloud.Cloud) error
	// UpdateCloud updates the definition of a current cloud.
	UpdateCloud(ctx context.Context, cld cloud.Cloud) error
	// DeleteCloud removes a cloud, and any permissions associated with it.
	DeleteCloud(ctx context.Context, name string) error
}

// CloudAccessService provides access to cloud permissions.
type CloudAccessService interface {
	// ReadUserAccessLevelForTarget returns the access level for the provided
	// subject (user) for the given target (cloud).
	ReadUserAccessLevelForTarget(ctx context.Context, subject user.Name, target corepermission.ID) (corepermission.Access, error)
	// ReadAllUserAccessForTarget  returns the user access for all users for
	// the given target (cloud).
	ReadAllUserAccessForTarget(ctx context.Context, target corepermission.ID) ([]corepermission.UserAccess, error)
	// CreatePermission sets the access level for a user on the given cloud.
	CreatePermission(ctx context.Context, spec corepermission.UserAccessSpec) (corepermission.UserAccess, error)
	// UpdatePermission updates the access level for a user on the given cloud.
	UpdatePermission(ctx context.Context, args access.UpdatePermissionArgs) error
	// ReadAllAccessForUserAndObjectType returns UserAccess for the given
	// subject (user) for all clouds based on objectType.
	ReadAllAccessForUserAndObjectType(ctx context.Context, subject user.Name, objectType corepermission.ObjectType) ([]corepermission.UserAccess, error)
	// AllModelAccessForCloudCredential for a given (cloud) credential key, return all
	// model name and model access levels.
	AllModelAccessForCloudCredential(ctx context.Context, key credential.Key) ([]access.CredentialOwnerModelAccess, error)
}

// CredentialService provides access to the credential domain service.
type CredentialService interface {
	CloudCredential(ctx context.Context, key credential.Key) (cloud.Credential, error)
	AllCloudCredentialsForOwner(ctx context.Context, owner user.Name) (map[credential.Key]cloud.Credential, error)
	CloudCredentialsForOwner(ctx context.Context, owner user.Name, cloudName string) (map[string]cloud.Credential, error)
	UpdateCloudCredential(ctx context.Context, key credential.Key, cred cloud.Credential) error
	RemoveCloudCredential(ctx context.Context, key credential.Key) error
	WatchCredential(ctx context.Context, key credential.Key) (watcher.NotifyWatcher, error)
	CheckAndUpdateCredential(ctx context.Context, key credential.Key, cred cloud.Credential, force bool) ([]credentialservice.UpdateCredentialModelResult, error)
	CheckAndRevokeCredential(ctx context.Context, key credential.Key, force bool) error
}
