// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"time"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/storage"
	"github.com/juju/names"
)

var (
	NewManagedFilesystemSource = &newManagedFilesystemSource
)

type Schedule struct {
	*schedule
}

func (s Schedule) Next() <-chan time.Time {
	return s.next()
}

func (s Schedule) Ready(t time.Time) ([]storage.VolumeParams, []storage.VolumeAttachmentParams) {
	return s.ready(t)
}

func (s Schedule) AddVolume(v storage.VolumeParams, t time.Time) {
	s.addVolume(v, t)
}

func (s Schedule) RemoveVolume(tag names.VolumeTag) {
	s.removeVolume(tag)
}

func (s Schedule) AddVolumeAttachment(a storage.VolumeAttachmentParams, t time.Time) {
	s.addVolumeAttachment(a, t)
}

func (s Schedule) RemoveVolumeAttachment(id params.MachineStorageId) {
	s.removeVolumeAttachment(id)
}

func NewSchedule(c Clock) Schedule {
	return Schedule{newSchedule(c)}
}
