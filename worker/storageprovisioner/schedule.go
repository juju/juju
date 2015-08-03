// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"container/heap"
	"time"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/storage"
	"github.com/juju/names"
)

// schedule provides a schedule for storage operations, with the following
// properties:
//  - fast to add and remove items: O(log(n)); n is the total number of items
//  - fast to identify/remove the next scheduled event: O(log(n))
type schedule struct {
	items                    scheduleItems
	pendingVolumes           map[names.VolumeTag]*pendingVolumeParams
	pendingVolumeAttachments map[params.MachineStorageId]*pendingVolumeAttachmentParams
}

// newSchedule constructs a new schedule.
func newSchedule() *schedule {
	return &schedule{
		pendingVolumes:           make(map[names.VolumeTag]*pendingVolumeParams),
		pendingVolumeAttachments: make(map[params.MachineStorageId]*pendingVolumeAttachmentParams),
	}
}

// next returns a channel which will send after the next scheduled item's time
// has been reached. If there are no scheduled items, nil is returned.
func (s *schedule) next() <-chan time.Time {
	if len(s.items) > 0 {
		return time.After(s.items[0].t.Sub(time.Now()))
	}
	return nil
}

// ready returns the parameters for pending operations that are scheduled at
// or before "now", and removes them from the schedule.
func (s *schedule) ready(now time.Time) ([]storage.VolumeParams, []storage.VolumeAttachmentParams) {
	var volumes []storage.VolumeParams
	var volumeAttachments []storage.VolumeAttachmentParams
	for len(s.items) > 0 && !s.items[0].t.After(now) {
		item := heap.Pop(&s.items).(*scheduleItem)
		switch pending := item.ptr.(type) {
		case *pendingVolumeParams:
			volumes = append(volumes, pending.VolumeParams)
			delete(s.pendingVolumes, pending.Tag)
		case *pendingVolumeAttachmentParams:
			volumeAttachments = append(volumeAttachments, pending.VolumeAttachmentParams)
			delete(s.pendingVolumeAttachments, params.MachineStorageId{
				MachineTag:    pending.Machine.String(),
				AttachmentTag: pending.Volume.String(),
			})
		}
	}
	return volumes, volumeAttachments
}

// addVolume schedules the creation of a volume at the given time.
func (s *schedule) addVolume(v storage.VolumeParams, t time.Time) {
	if _, ok := s.pendingVolumes[v.Tag]; ok {
		panic(errors.Errorf("volume %s is already scheduled", v.Tag.Id()))
	}
	pending := &pendingVolumeParams{VolumeParams: v}
	pending.scheduleItem = scheduleItem{t: t, ptr: pending}
	s.pendingVolumes[v.Tag] = pending
	heap.Push(&s.items, &pending.scheduleItem)
}

// removeVolume removes the creation of the identified volume from the schedule.
func (s *schedule) removeVolume(tag names.VolumeTag) {
	if pending, ok := s.pendingVolumes[tag]; ok {
		heap.Remove(&s.items, pending.scheduleItem.i)
		delete(s.pendingVolumes, tag)
	}
}

// addVolumeAttachment schedules the creation of a volume attachment at the
// given time.
func (s *schedule) addVolumeAttachment(v storage.VolumeAttachmentParams, t time.Time) {
	id := params.MachineStorageId{
		MachineTag:    v.Machine.String(),
		AttachmentTag: v.Volume.String(),
	}
	if _, ok := s.pendingVolumeAttachments[id]; ok {
		panic(errors.Errorf("volume attachment %v is already scheduled", id))
	}
	pending := &pendingVolumeAttachmentParams{VolumeAttachmentParams: v}
	pending.scheduleItem = scheduleItem{t: t, ptr: pending}
	s.pendingVolumeAttachments[id] = pending
	heap.Push(&s.items, &pending.scheduleItem)
}

// removeVolumeAttachment removes the creation of the identified volume
// attachment from the schedule.
func (s *schedule) removeVolumeAttachment(id params.MachineStorageId) {
	if pending, ok := s.pendingVolumeAttachments[id]; ok {
		heap.Remove(&s.items, pending.scheduleItem.i)
		delete(s.pendingVolumeAttachments, id)
	}
}

type pendingVolumeParams struct {
	storage.VolumeParams
	scheduleItem scheduleItem
}

type pendingVolumeAttachmentParams struct {
	storage.VolumeAttachmentParams
	scheduleItem scheduleItem
}

/////////////////////////////////////////////////

type scheduleItems []*scheduleItem

type scheduleItem struct {
	i   int
	t   time.Time
	ptr interface{}
}

func (s scheduleItems) Len() int {
	return len(s)
}

func (s scheduleItems) Less(i, j int) bool {
	return s[i].t.Before(s[j].t)
}

func (s scheduleItems) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
	s[i].i = i
	s[j].i = j
}

func (s *scheduleItems) Push(x interface{}) {
	item := x.(*scheduleItem)
	item.i = len(*s)
	*s = append(*s, item)
}

func (s *scheduleItems) Pop() interface{} {
	n := len(*s) - 1
	x := (*s)[n]
	*s = (*s)[:n]
	return x
}
