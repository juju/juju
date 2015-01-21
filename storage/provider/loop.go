// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"path/filepath"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/storage"
)

const (
	// Loop provider types
	LoopProviderType     = storage.ProviderType("loop")
	HostLoopProviderType = storage.ProviderType("hostloop")

	// Config attributes
	LoopDataDir = "data-dir" // top level directory where loop devices are created.
	LoopSubDir  = "sub-dir"  // optional subdirectory for loop devices.
)

// loopProviders create volume sources which use loop devices.
type loopProvider struct{}

var _ storage.Provider = (*loopProvider)(nil)

// ValidateConfig is defined on the Provider interface.
func (lp *loopProvider) ValidateConfig(providerConfig *storage.Config) error {
	dataDir, ok := providerConfig.ValueString(LoopDataDir)
	if !ok || dataDir == "" {
		return errors.New("no data directory specified")
	}
	return nil
}

// VolumeSource is defined on the Provider interface.
func (lp *loopProvider) VolumeSource(environConfig *config.Config, providerConfig *storage.Config) (storage.VolumeSource, error) {
	if err := lp.ValidateConfig(providerConfig); err != nil {
		return nil, err
	}
	dataDir, _ := providerConfig.ValueString(LoopDataDir)
	subDir, _ := providerConfig.ValueString(LoopDataDir)
	return &loopVolumeSource{
		dataDir,
		subDir,
	}, nil
}

// loopVolumeSource provides common functionality to handle
// loop devices for rootfs and host loop volume sources.
type loopVolumeSource struct {
	dataDir string
	subDir  string
}

var _ storage.VolumeSource = (*loopVolumeSource)(nil)

func (lvs *loopVolumeSource) rootDeviceDir() string {
	dirParts := []string{lvs.dataDir}
	dirParts = append(dirParts, strings.Split(lvs.subDir, "/")...)
	return filepath.Join(dirParts...)
}

func (lvs *loopVolumeSource) CreateVolumes(params []storage.VolumeParams) ([]storage.BlockDevice, error) {
	panic("not implemented")
}

func (lvs *loopVolumeSource) DescribeVolumes(volIds []string) ([]storage.BlockDevice, error) {
	panic("not implemented")
}

func (lvs *loopVolumeSource) DestroyVolumes(volIds []string) error {
	panic("not implemented")
}

func (lvs *loopVolumeSource) ValidateVolumeParams(params storage.VolumeParams) error {
	panic("not implemented")
}

func (lvs *loopVolumeSource) AttachVolumes(volIds []string, instId []instance.Id) error {
	panic("not implemented")
}

func (lvs *loopVolumeSource) DetachVolumes(volIds []string, instId []instance.Id) error {
	panic("not implemented")
}
