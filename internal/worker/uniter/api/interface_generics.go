// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"context"
	"time"

	"github.com/juju/names/v4"

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
	Unit(tag names.UnitTag) (Unit, error)
	Action(tag names.ActionTag) (*uniter.Action, error)
	Application(tag names.ApplicationTag) (Application, error)
	ActionStatus(tag names.ActionTag) (string, error)
	Relation(tag names.RelationTag) (Relation, error)
	RelationById(int) (Relation, error)
	Model() (*model.Model, error)
	ModelConfig(context.Context) (*config.Config, error)
	UnitStorageAttachments(unitTag names.UnitTag) ([]params.StorageAttachmentId, error)
	StorageAttachment(storageTag names.StorageTag, unitTag names.UnitTag) (params.StorageAttachment, error)
	GoalState() (application.GoalState, error)
	CloudSpec() (*params.CloudSpec, error)
	ActionBegin(tag names.ActionTag) error
	ActionFinish(tag names.ActionTag, status string, results map[string]interface{}, message string) error
	UnitWorkloadVersion(tag names.UnitTag) (string, error)
	SetUnitWorkloadVersion(tag names.UnitTag, version string) error
	OpenedMachinePortRangesByEndpoint(machineTag names.MachineTag) (map[names.UnitTag]network.GroupedPortRanges, error)
	OpenedPortRangesByEndpoint() (map[names.UnitTag]network.GroupedPortRanges, error)
	LeadershipSettings() uniter.LeadershipSettingsAccessor
	SLALevel() (string, error)
	CloudAPIVersion() (string, error)
	APIAddresses() ([]string, error)
	WatchRelationUnits(names.RelationTag, names.UnitTag) (watcher.RelationUnitsWatcher, error)
	WatchStorageAttachment(names.StorageTag, names.UnitTag) (watcher.NotifyWatcher, error)
	WatchUpdateStatusHookInterval() (watcher.NotifyWatcher, error)
	UpdateStatusHookInterval() (time.Duration, error)
	StorageAttachmentLife([]params.StorageAttachmentId) ([]params.LifeResult, error)
}
