// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/plans/common"
)

func NewLocalPlan() common.Plan {
	return &localPlan{}
}

type localPlan struct{}

func (i *localPlan) AttachVolume(volumeInfo map[string]string) (storage.BlockDevice, error) {
	return storage.BlockDevice{}, nil
}

func (i *localPlan) DetachVolume(volumeInfo map[string]string) error {
	return nil
}
