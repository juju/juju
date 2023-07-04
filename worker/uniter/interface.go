// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"time"

	"github.com/juju/names/v4"

	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/worker/uniter/domain"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/runner/context"
	"github.com/juju/juju/worker/uniter/storage"
)

// A UniterExecutionObserver gets the appropriate methods called when a hook
// is executed and either succeeds or fails.  Missing hooks don't get reported
// in this way.
type UniterExecutionObserver interface {
	HookCompleted(hookName string)
	HookFailed(hookName string)
}

// RebootQuerier is implemented by types that can deliver one-off machine
// reboot notifications to entities.
type RebootQuerier interface {
	Query(tag names.Tag) (bool, error)
}

// SecretsClient provides methods used by the remote state watcher, hook context,
// and op callbacks.
type SecretsClient interface {
	remotestate.SecretsClient
	context.SecretsAccessor
}

// UniterClient provides methods used by the uniter api facade client.
type UniterClient interface {
	context.UniterClient
	storage.StorageAccessor
	Unit(tag names.UnitTag) (domain.Unit, error)
	Action(tag names.ActionTag) (*uniter.Action, error)
	Application(tag names.ApplicationTag) (domain.Application, error)
	ActionStatus(tag names.ActionTag) (string, error)
	Relation(tag names.RelationTag) (domain.Relation, error)
	RelationById(int) (domain.Relation, error)
	Model() (*model.Model, error)
	ModelConfig() (*config.Config, error)
	SLALevel() (string, error)
	CloudAPIVersion() (string, error)
	APIAddresses() ([]string, error)
	WatchRelationUnits(names.RelationTag, names.UnitTag) (watcher.RelationUnitsWatcher, error)
	WatchStorageAttachment(names.StorageTag, names.UnitTag) (watcher.NotifyWatcher, error)
	WatchUpdateStatusHookInterval() (watcher.NotifyWatcher, error)
	UpdateStatusHookInterval() (time.Duration, error)
	StorageAttachmentLife([]params.StorageAttachmentId) ([]params.LifeResult, error)
}
