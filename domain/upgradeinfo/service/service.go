// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/juju/domain"
	uistate "github.com/juju/juju/domain/upgradeinfo/state"
	"github.com/juju/version/v2"
)

// State describes retrieval and persistence
// methods for upgrade info.
type State interface {
	EnsureUpgradeInfo(context.Context, string, version.Number, version.Number) (uistate.Info, []uistate.InfoControllerNode, error)
	IsUpgrading(context.Context) (bool, error)
}

// Service provides the API for working with upgrade info
type Service struct {
	st State
}

func NewService(st State) *Service {
	return &Service{st}
}

// EnsureUpgradeInfo returns an Info describing a current upgrade.
// If a matching upgrade is in progress, that upgrade is returned
func (s *Service) EnsureUpgradeInfo(ctx context.Context, controllerID string, previousVersion, targetVersion version.Number) (Info, error) {
	info, nodeInfos, err := s.st.EnsureUpgradeInfo(ctx, controllerID, previousVersion, targetVersion)
	if err != nil {
		return Info{}, errors.Trace(err)
	}
	initTime, err := time.Parse(time.RFC3339, info.InitTime)
	if err != nil {
		return Info{}, errors.Annotatef(err, "failed to parse init time %q", info.InitTime)
	}
	var startTime time.Time
	if info.StartTime != "" {
		startTime, err = time.Parse(time.RFC3339, info.StartTime)
		if err != nil {
			return Info{}, errors.Annotatef(err, "failed to parse start time %q", info.StartTime)
		}
	}
	controllersReady := set.NewStrings()
	controllersDone := set.NewStrings()
	for _, nodeInfo := range nodeInfos {
		if nodeInfo.NodeStatus == "done" {
			controllersDone.Add(nodeInfo.ControllerNodeID)
		} else if nodeInfo.NodeStatus == "ready" {
			controllersReady.Add(nodeInfo.ControllerNodeID)
		}
	}
	res := Info{
		// NOTE: We use the function params since we know
		// these will not have changed so there is no need
		// to parse them from info
		PreviousVersion:  previousVersion,
		TargetVersion:    targetVersion,
		InitTime:         initTime,
		StartTime:        startTime,
		ControllersReady: controllersReady.SortedValues(),
		ControllersDone:  controllersDone.SortedValues(),
	}
	return res, nil
}

// Watch returns a watcher for the state underlying the current
// UpgradeInfo instance. This is provided purely for convenience.
func (s *Service) Watch(ctx context.Context) domain.NotifyWatcher {
	// TODO (jack-w-shaw) Waiting until we can do watchers
	return nil
}

// AllProvisionedControllersReady returns true if and only if all controllers
// that have been started by the provisioner have called EnsureUpgradeInfo with
// matching versions.
func (s *Service) AllProvisionedControllersReady(ctx context.Context) (bool, error) {
	// TODO (jack-w-shaw) this
	return false, nil
}

// StartUpgrade starts the current upgrade if it exists
func (s *Service) StartUpgrade(ctx context.Context) error {
	// TODO (jack-w-shaw) this
	return nil
}

// SetControllerDone marks the supplied state controllerId as having
// completed its upgrades. When SetControllerDone is called by the
// last provisioned controller, the current upgrade info document
// will be archived with a status of UpgradeComplete.
func (s *Service) SetControllerDone(ctx context.Context, controllerID string) error {
	// TODO (jack-w-shaw) this
	return nil
}

// IsUpgrading returns true if an upgrade is currently in progress.
func (s *Service) IsUpgrading(ctx context.Context) (bool, error) {
	upgrading, err := s.st.IsUpgrading(ctx)
	return upgrading, errors.Trace(err)
}
