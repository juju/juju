// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"context"

	"github.com/juju/names/v6"

	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/rpc/params"
)

// TODO(wallyworld) - mockgen breaks on WatchRelationUnits method due to generics.
// The generated mock file needed to be edited manually to fix the error(s).
// Typo below is deliberate to avoid mock linting from failing.

// ProviderIDGetter defines the API to get provider ID.
type ProviderIDGetter interface {
	ProviderID() string
	Refresh(ctx context.Context) error
	Name() string
}

// Unit defines the methods on uniter.api.Unit.
type Unit interface {
	ProviderIDGetter
	Life() life.Value
	Refresh(context.Context) error
	ApplicationTag() names.ApplicationTag
	EnsureDead(context.Context) error
	ClearResolved(context.Context) error
	DestroyAllSubordinates(context.Context) error
	HasSubordinates(context.Context) (bool, error)
	LXDProfileName(context.Context) (string, error)
	CanApplyLXDProfile(context.Context) (bool, error)
	CharmURL(context.Context) (string, error)
	Watch(context.Context) (watcher.NotifyWatcher, error)

	// Used by runner.context.

	ApplicationName() string
	ConfigSettings(context.Context) (charm.Settings, error)
	LogActionMessage(context.Context, names.ActionTag, string) error
	Name() string
	NetworkInfo(ctx context.Context, bindings []string, relationId *int) (map[string]params.NetworkInfoResult, error)
	RequestReboot(context.Context) error
	SetUnitStatus(ctx context.Context, unitStatus status.Status, info string, data map[string]interface{}) error
	SetAgentStatus(ctx context.Context, agentStatus status.Status, info string, data map[string]interface{}) error
	State(ctx context.Context) (params.UnitStateResult, error)
	SetState(ctx context.Context, unitState params.SetUnitStateArg) error
	Tag() names.UnitTag
	UnitStatus(context.Context) (params.StatusResult, error)
	CommitHookChanges(context.Context, params.CommitHookChangesArgs) error
	PublicAddress(context.Context) (string, error)
	PrincipalName(context.Context) (string, bool, error)
	AssignedMachine(context.Context) (names.MachineTag, error)
	AvailabilityZone(context.Context) (string, error)
	PrivateAddress(context.Context) (string, error)
	Resolved(context.Context) (params.ResolvedMode, error)

	// Used by remotestate watcher.

	WatchConfigSettingsHash(context.Context) (watcher.StringsWatcher, error)
	WatchTrustConfigSettingsHash(context.Context) (watcher.StringsWatcher, error)
	WatchRelations(context.Context) (watcher.StringsWatcher, error)
	WatchResolveMode(context.Context) (watcher.NotifyWatcher, error)
	WatchAddressesHash(context.Context) (watcher.StringsWatcher, error)
	WatchActionNotifications(context.Context) (watcher.StringsWatcher, error)
	WatchStorage(context.Context) (watcher.StringsWatcher, error)
	WatchInstanceData(context.Context) (watcher.NotifyWatcher, error)

	// Used by relationer.

	Application(context.Context) (Application, error)
	RelationsStatus(context.Context) ([]uniter.RelationStatus, error)
	Destroy(context.Context) error

	// Used by operation.Callbacks.

	SetCharm(ctx context.Context, curl string) error
}

// Application defines the methods on uniter.api.Application.
type Application interface {
	Life() life.Value
	Tag() names.ApplicationTag
	Status(ctx context.Context, unitName string) (params.ApplicationStatusResult, error)
	SetStatus(ctx context.Context, unitName string, appStatus status.Status, info string, data map[string]interface{}) error
	CharmModifiedVersion(context.Context) (int, error)
	CharmURL(context.Context) (string, bool, error)

	// Used by remotestate watcher.

	Watch(context.Context) (watcher.NotifyWatcher, error)
	Refresh(context.Context) error
}

// Relation defines the methods on uniter.api.Relation.
type Relation interface {
	Endpoint(context.Context) (*uniter.Endpoint, error)
	Id() int
	Life() life.Value
	OtherApplication() string
	OtherModelUUID() string
	Refresh(context.Context) error
	SetStatus(ctx context.Context, status2 relation.Status) error
	String() string
	Suspended() bool
	Tag() names.RelationTag
	Unit(context.Context, names.UnitTag) (RelationUnit, error)
	UpdateSuspended(bool)
}

// RelationUnit defines the methods on uniter.api.RelationUnit.
type RelationUnit interface {
	ApplicationSettings(context.Context) (*uniter.Settings, error)
	Endpoint() uniter.Endpoint
	EnterScope(context.Context) error
	LeaveScope(context.Context) error
	Relation() Relation
	ReadSettings(ctx context.Context, name string) (params.Settings, error)
	Settings(context.Context) (*uniter.Settings, error)
}

// Charm defines the methods on uniter.api.Charm.
type Charm interface {
	URL() string
	LXDProfileRequired(context.Context) (bool, error)
	ArchiveSha256(context.Context) (string, error)
}

// SecretsAccessor is used by the hook context to access the secrets backend.
type SecretsAccessor interface {
	CreateSecretURIs(context.Context, int) ([]*secrets.URI, error)
	SecretMetadata(context.Context) ([]secrets.SecretOwnerMetadata, error)
	SecretRotated(ctx context.Context, uri string, oldRevision int) error
}

// SecretsWatcher is used by the remote state watcher.
type SecretsWatcher interface {
	WatchConsumedSecretsChanges(ctx context.Context, unitName string) (watcher.StringsWatcher, error)
	GetConsumerSecretsRevisionInfo(context.Context, string, []string) (map[string]secrets.SecretRevisionInfo, error)
	WatchObsolete(ctx context.Context, ownerTags ...names.Tag) (watcher.StringsWatcher, error)
}

// SecretsBackend provides access to a secrets backend.
type SecretsBackend interface {
	GetContent(ctx context.Context, uri *secrets.URI, label string, refresh, peek bool) (secrets.SecretValue, error)
	SaveContent(ctx context.Context, uri *secrets.URI, revision int, value secrets.SecretValue) (secrets.ValueRef, error)
	DeleteContent(ctx context.Context, uri *secrets.URI, revision int) error
	DeleteExternalContent(ctx context.Context, ref secrets.ValueRef) error
}

// SecretsClient provides access to the secrets manager facade.
type SecretsClient interface {
	SecretsWatcher
	SecretsAccessor
}

// StorageAccessor is an interface for accessing information about
// storage attachments.
type StorageAccessor interface {
	StorageAttachment(context.Context, names.StorageTag, names.UnitTag) (params.StorageAttachment, error)
	UnitStorageAttachments(context.Context, names.UnitTag) ([]params.StorageAttachmentId, error)
	DestroyUnitStorageAttachments(context.Context, names.UnitTag) error
	RemoveStorageAttachment(context.Context, names.StorageTag, names.UnitTag) error
}
