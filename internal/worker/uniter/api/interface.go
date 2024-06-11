// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	stdcontext "context"

	"github.com/juju/names/v5"

	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
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
	Refresh(ctx stdcontext.Context) error
	Name() string
}

// Unit defines the methods on uniter.api.Unit.
type Unit interface {
	ProviderIDGetter
	Life() life.Value
	Refresh(stdcontext.Context) error
	ApplicationTag() names.ApplicationTag
	EnsureDead() error
	ClearResolved() error
	DestroyAllSubordinates() error
	HasSubordinates() (bool, error)
	LXDProfileName() (string, error)
	CanApplyLXDProfile() (bool, error)
	CharmURL() (string, error)
	Watch(stdcontext.Context) (watcher.NotifyWatcher, error)

	// Used by runner.context.

	ApplicationName() string
	ConfigSettings() (charm.Settings, error)
	LogActionMessage(names.ActionTag, string) error
	Name() string
	NetworkInfo(bindings []string, relationId *int) (map[string]params.NetworkInfoResult, error)
	RequestReboot() error
	SetUnitStatus(ctx stdcontext.Context, unitStatus status.Status, info string, data map[string]interface{}) error
	SetAgentStatus(agentStatus status.Status, info string, data map[string]interface{}) error
	State(ctx stdcontext.Context) (params.UnitStateResult, error)
	SetState(ctx stdcontext.Context, unitState params.SetUnitStateArg) error
	Tag() names.UnitTag
	UnitStatus(stdcontext.Context) (params.StatusResult, error)
	CommitHookChanges(params.CommitHookChangesArgs) error
	PublicAddress() (string, error)
	PrincipalName() (string, bool, error)
	AssignedMachine() (names.MachineTag, error)
	AvailabilityZone() (string, error)
	PrivateAddress() (string, error)
	Resolved() params.ResolvedMode

	// Used by remotestate watcher.

	WatchConfigSettingsHash() (watcher.StringsWatcher, error)
	WatchTrustConfigSettingsHash() (watcher.StringsWatcher, error)
	WatchRelations() (watcher.StringsWatcher, error)
	WatchAddressesHash() (watcher.StringsWatcher, error)
	WatchUpgradeSeriesNotifications(ctx stdcontext.Context) (watcher.NotifyWatcher, error)
	WatchActionNotifications() (watcher.StringsWatcher, error)
	WatchStorage() (watcher.StringsWatcher, error)
	WatchInstanceData() (watcher.NotifyWatcher, error)
	UpgradeSeriesStatus(ctx stdcontext.Context) (model.UpgradeSeriesStatus, string, error)

	// Used by relationer.

	Application(stdcontext.Context) (Application, error)
	RelationsStatus() ([]uniter.RelationStatus, error)
	Destroy() error

	// Used by operation.Callbacks.

	SetUpgradeSeriesStatus(ctx stdcontext.Context, upgradeSeriesStatus model.UpgradeSeriesStatus, reason string) error
	SetCharmURL(curl string) error
}

// Application defines the methods on uniter.api.Application.
type Application interface {
	Life() life.Value
	Tag() names.ApplicationTag
	Status(unitName string) (params.ApplicationStatusResult, error)
	SetStatus(unitName string, appStatus status.Status, info string, data map[string]interface{}) error
	CharmModifiedVersion() (int, error)
	CharmURL() (string, bool, error)

	// Used by remotestate watcher.

	WatchLeadershipSettings() (watcher.NotifyWatcher, error)
	Watch(ctx stdcontext.Context) (watcher.NotifyWatcher, error)
	Refresh(stdcontext.Context) error
}

// Relation defines the methods on uniter.api.Relation.
type Relation interface {
	Endpoint(stdcontext.Context) (*uniter.Endpoint, error)
	Id() int
	Life() life.Value
	OtherApplication() string
	Refresh(stdcontext.Context) error
	SetStatus(ctx stdcontext.Context, status2 relation.Status) error
	String() string
	Suspended() bool
	Tag() names.RelationTag
	Unit(stdcontext.Context, names.UnitTag) (RelationUnit, error)
	UpdateSuspended(bool)
}

// RelationUnit defines the methods on uniter.api.RelationUnit.
type RelationUnit interface {
	ApplicationSettings() (*uniter.Settings, error)
	Endpoint() uniter.Endpoint
	EnterScope() error
	LeaveScope() error
	Relation() Relation
	ReadSettings(name string) (params.Settings, error)
	Settings() (*uniter.Settings, error)
}

// Charm defines the methods on uniter.api.Charm.
type Charm interface {
	URL() string
	LXDProfileRequired() (bool, error)
	ArchiveSha256() (string, error)
}

// SecretsAccessor is used by the hook context to access the secrets backend.
type SecretsAccessor interface {
	CreateSecretURIs(int) ([]*secrets.URI, error)
	SecretMetadata() ([]secrets.SecretOwnerMetadata, error)
	SecretRotated(uri string, oldRevision int) error
}

// SecretsWatcher is used by the remote state watcher.
type SecretsWatcher interface {
	WatchConsumedSecretsChanges(unitName string) (watcher.StringsWatcher, error)
	GetConsumerSecretsRevisionInfo(string, []string) (map[string]secrets.SecretRevisionInfo, error)
	WatchObsolete(ownerTags ...names.Tag) (watcher.StringsWatcher, error)
}

// SecretsBackend provides access to a secrets backend.
type SecretsBackend interface {
	GetContent(uri *secrets.URI, label string, refresh, peek bool) (secrets.SecretValue, error)
	SaveContent(uri *secrets.URI, revision int, value secrets.SecretValue) (secrets.ValueRef, error)
	DeleteContent(uri *secrets.URI, revision int) error
	DeleteExternalContent(ref secrets.ValueRef) error
}

// SecretsClient provides access to the secrets manager facade.
type SecretsClient interface {
	SecretsWatcher
	SecretsAccessor
}

// StorageAccessor is an interface for accessing information about
// storage attachments.
type StorageAccessor interface {
	StorageAttachment(names.StorageTag, names.UnitTag) (params.StorageAttachment, error)
	UnitStorageAttachments(names.UnitTag) ([]params.StorageAttachmentId, error)
	DestroyUnitStorageAttachments(names.UnitTag) error
	RemoveStorageAttachment(names.StorageTag, names.UnitTag) error
}
