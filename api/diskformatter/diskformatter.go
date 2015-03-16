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
	tag    names.MachineTag
}

// NewState creates a new client-side DiskFormatter facade.
func NewState(caller base.APICaller, authTag names.MachineTag) *State {
	return &State{
		base.NewFacadeCaller(caller, diskFormatterFacade),
		authTag,
	}
}

// WatchAttachedVolumes watches for changes in the machine's volume
// attachments.
func (st *State) WatchAttachedVolumes() (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: st.tag.String()}},
	}
	err := st.facade.FacadeCall("WatchAttachedVolumes", args, &results)
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
	w := watcher.NewNotifyWatcher(st.facade.RawAPICaller(), result)
	return w, nil
}

// AttachedVolumes returns details of volumes attached to the machine
// running the authenticated agent.
func (st *State) AttachedVolumes() ([]params.VolumeAttachment, error) {
	args := params.Entities{
		Entities: []params.Entity{{st.tag.String()}},
	}
	var results params.VolumeAttachmentsResults
	err := st.facade.FacadeCall("AttachedVolumes", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		panic(errors.Errorf("expected 1 result, got %d", len(results.Results)))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	return result.Attachments, nil
}

// VolumePreparationInfo returns the information required to format the
// specified volumes.
func (st *State) VolumePreparationInfo(tags []names.VolumeTag) ([]params.VolumePreparationInfoResult, error) {
	var results params.VolumePreparationInfoResults
	args := params.MachineStorageIds{
		Ids: make([]params.MachineStorageId, len(tags)),
	}
	for i, tag := range tags {
		args.Ids[i].MachineTag = st.tag.String()
		args.Ids[i].EntityTag = tag.String()
	}
	err := st.facade.FacadeCall("VolumePreparationInfo", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != len(args.Ids) {
		panic(errors.Errorf("expected %d results, got %d", len(args.Ids), len(results.Results)))
	}
	return results.Results, nil
}
