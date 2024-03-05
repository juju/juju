// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/internal/storage/plans/common"
)

func NewLocalPlan() common.Plan {
	return &localPlan{}
}

type localPlan struct{}

func (i *localPlan) AttachVolume(volumeInfo map[string]string) (blockdevice.BlockDevice, error) {
	return blockdevice.BlockDevice{}, nil
}

func (i *localPlan) DetachVolume(volumeInfo map[string]string) error {
	return nil
}
