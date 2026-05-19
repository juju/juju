// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

//go:generate go run github.com/canonical/gomock/mockgen -package storageprovisioner -destination storageprovisioner_mock_test.go github.com/juju/juju/internal/worker/storageprovisioner VolumeAccessor,FilesystemAccessor,MachineAccessor,LifecycleManager,StatusSetter
