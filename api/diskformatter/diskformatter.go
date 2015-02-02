// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskformatter

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
)

const diskFormatterFacade = "DiskFormatter"

// State provides access to a diskformatter worker's view of the state.
type State struct {
	facade base.FacadeCaller
	tag    names.UnitTag
}

// NewState creates a new client-side DiskFormatter facade.
func NewState(caller base.APICaller, authTag names.UnitTag) *State {
	return &State{
		base.NewFacadeCaller(caller, diskFormatterFacade),
		authTag,
	}
}

// WatchBlockDevices watches the block devices attached to the machine
// that hosts the authenticated unit agent.
func (st *State) WatchBlockDevices() (watcher.StringsWatcher, error) {
	var results params.StringsWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: st.tag.String()}},
	}
	err := st.facade.FacadeCall("WatchBlockDevices", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		panic(errors.Errorf("expected 1 result, got %d", len(results.Results)))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := watcher.NewStringsWatcher(st.facade.RawAPICaller(), result)
	return w, nil
}

// BlockDevices returns details of attached block devices with the specified tags.
func (st *State) BlockDevices(tags []names.DiskTag) (params.BlockDeviceResults, error) {
	var result params.BlockDeviceResults
	args := params.Entities{
		Entities: make([]params.Entity, len(tags)),
	}
	for i, tag := range tags {
		args.Entities[i].Tag = tag.String()
	}
	err := st.facade.FacadeCall("BlockDevices", args, &result)
	if err != nil {
		return params.BlockDeviceResults{}, err
	}
	if len(result.Results) != len(tags) {
		panic(errors.Errorf("expected %d results, got %d", len(tags), len(result.Results)))
	}
	return result, nil
}

// BlockDeviceStorageInstances returns the details of storage instance that
// each named block device is assigned to.
func (st *State) BlockDeviceStorageInstances(tags []names.DiskTag) (params.StorageInstanceResults, error) {
	var results params.StorageInstanceResults
	args := params.Entities{
		Entities: make([]params.Entity, len(tags)),
	}
	for i, tag := range tags {
		args.Entities[i].Tag = tag.String()
	}
	err := st.facade.FacadeCall("BlockDeviceStorageInstances", args, &results)
	if err != nil {
		return params.StorageInstanceResults{}, err
	}
	if len(results.Results) != len(tags) {
		panic(errors.Errorf("expected %d results, got %d", len(tags), len(results.Results)))
	}
	return results, nil
}
