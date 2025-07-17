// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"testing"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"
)

type registerSuite struct{}

func TestRegisterSuite(t *testing.T) {
	tc.Run(t, &registerSuite{})
}

func (s *registerSuite) TestRegister(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	registry := NewMockFacadeRegistry(ctrl)
	registry.EXPECT().MustRegister("StorageProvisioner", 4, gomock.Any(), gomock.Any()).AnyTimes()
	registry.EXPECT().MustRegister("VolumeAttachmentsWatcher", 2, gomock.Any(), gomock.Any()).AnyTimes()
	registry.EXPECT().MustRegister("VolumeAttachmentPlansWatcher", 1, gomock.Any(), gomock.Any()).AnyTimes()
	registry.EXPECT().MustRegister("FilesystemAttachmentsWatcher", 2, gomock.Any(), gomock.Any()).AnyTimes()

	Register(registry)
}
