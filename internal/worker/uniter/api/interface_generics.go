// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"context"
	"time"

	"github.com/juju/names/v5"

	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
)

// UniterClient provides methods used by the uniter api facade client.
type UniterClient interface {
	StorageAccessor
	Charm(curl string) (Charm, error)
	Unit(ctx context.Context, tag names.UnitTag) (Unit, error)
	Action(ctx context.Context, tag names.ActionTag) (*uniter.Action, error)
	Application(ctx context.Context, tag names.ApplicationTag) (Application, error)
	ActionStatus(ctx context.Context, tag names.ActionTag) (string, error)
	Relation(ctx context.Context, tag names.RelationTag) (Relation, error)
	RelationById(context.Context, int) (Relation, error)
	Model(context.Context) (*model.Model, error)
	ModelConfig(context.Context) (*config.Config, error)
	UnitStorageAttachments(unitTag names.UnitTag) ([]params.StorageAttachmentId, error)
	StorageAttachment(storageTag names.StorageTag, unitTag names.UnitTag) (params.StorageAttachment, error)
	GoalState(context.Context) (application.GoalState, error)
	CloudSpec(context.Context) (*params.CloudSpec, error)
	ActionBegin(ctx context.Context, tag names.ActionTag) error
	ActionFinish(ctx context.Context, tag names.ActionTag, status string, results map[string]interface{}, message string) error
	UnitWorkloadVersion(ctx context.Context, tag names.UnitTag) (string, error)
	SetUnitWorkloadVersion(ctx context.Context, tag names.UnitTag, version string) error
	OpenedMachinePortRangesByEndpoint(ctx context.Context, machineTag names.MachineTag) (map[names.UnitTag]network.GroupedPortRanges, error)
	OpenedPortRangesByEndpoint(ctx context.Context) (map[names.UnitTag]network.GroupedPortRanges, error)
	LeadershipSettings() uniter.LeadershipSettingsAccessor
	SLALevel(context.Context) (string, error)
	CloudAPIVersion(context.Context) (string, error)
	APIAddresses() ([]string, error)
	WatchRelationUnits(context.Context, names.RelationTag, names.UnitTag) (watcher.RelationUnitsWatcher, error)
	WatchStorageAttachment(names.StorageTag, names.UnitTag) (watcher.NotifyWatcher, error)
	WatchUpdateStatusHookInterval() (watcher.NotifyWatcher, error)
	UpdateStatusHookInterval() (time.Duration, error)
	StorageAttachmentLife([]params.StorageAttachmentId) ([]params.LifeResult, error)
}
