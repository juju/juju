// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/set"
	"launchpad.net/tomb"

	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.storageprovisioner")

var newManagedFilesystemSource = provider.NewManagedFilesystemSource

// VolumeAccessor defines an interface used to allow a storage provisioner
// worker to perform volume related operations.
type VolumeAccessor interface {
	// WatchBlockDevices watches for changes to the block devices of the
	// specified machine.
	WatchBlockDevices(names.MachineTag) (apiwatcher.NotifyWatcher, error)

	// WatchVolumes watches for changes to volumes that this storage
	// provisioner is responsible for.
	WatchVolumes() (apiwatcher.StringsWatcher, error)

	// WatchVolumeAttachments watches for changes to volume attachments
	// that this storage provisioner is responsible for.
	WatchVolumeAttachments() (apiwatcher.MachineStorageIdsWatcher, error)

	// Volumes returns details of volumes with the specified tags.
	Volumes([]names.VolumeTag) ([]params.VolumeResult, error)

	// VolumeBlockDevices returns details of block devices corresponding to
	// the specified volume attachment IDs.
	VolumeBlockDevices([]params.MachineStorageId) ([]params.BlockDeviceResult, error)

	// VolumeAttachments returns details of volume attachments with
	// the specified tags.
	VolumeAttachments([]params.MachineStorageId) ([]params.VolumeAttachmentResult, error)

	// VolumeParams returns the parameters for creating the volumes
	// with the specified tags.
	VolumeParams([]names.VolumeTag) ([]params.VolumeParamsResult, error)

	// VolumeAttachmentParams returns the parameters for creating the
	// volume attachments with the specified tags.
	VolumeAttachmentParams([]params.MachineStorageId) ([]params.VolumeAttachmentParamsResult, error)

	// SetVolumeInfo records the details of newly provisioned volumes.
	SetVolumeInfo([]params.Volume) ([]params.ErrorResult, error)

	// SetVolumeAttachmentInfo records the details of newly provisioned
	// volume attachments.
	SetVolumeAttachmentInfo([]params.VolumeAttachment) ([]params.ErrorResult, error)
}

// FilesystemAccessor defines an interface used to allow a storage provisioner
// worker to perform filesystem related operations.
type FilesystemAccessor interface {
	// WatchFilesystems watches for changes to filesystems that this
	// storage provisioner is responsible for.
	WatchFilesystems() (apiwatcher.StringsWatcher, error)

	// WatchFilesystemAttachments watches for changes to filesystem attachments
	// that this storage provisioner is responsible for.
	WatchFilesystemAttachments() (apiwatcher.MachineStorageIdsWatcher, error)

	// Filesystems returns details of filesystems with the specified tags.
	Filesystems([]names.FilesystemTag) ([]params.FilesystemResult, error)

	// FilesystemAttachments returns details of filesystem attachments with
	// the specified tags.
	FilesystemAttachments([]params.MachineStorageId) ([]params.FilesystemAttachmentResult, error)

	// FilesystemParams returns the parameters for creating the filesystems
	// with the specified tags.
	FilesystemParams([]names.FilesystemTag) ([]params.FilesystemParamsResult, error)

	// FilesystemAttachmentParams returns the parameters for creating the
	// filesystem attachments with the specified tags.
	FilesystemAttachmentParams([]params.MachineStorageId) ([]params.FilesystemAttachmentParamsResult, error)

	// SetFilesystemInfo records the details of newly provisioned filesystems.
	SetFilesystemInfo([]params.Filesystem) ([]params.ErrorResult, error)

	// SetFilesystemAttachmentInfo records the details of newly provisioned
	// filesystem attachments.
	SetFilesystemAttachmentInfo([]params.FilesystemAttachment) ([]params.ErrorResult, error)
}

// MachineAccessor defines an interface used to allow a storage provisioner
// worker to perform machine related operations.
type MachineAccessor interface {
	// WatchMachine watches for changes to the specified machine.
	WatchMachine(names.MachineTag) (apiwatcher.NotifyWatcher, error)

	// InstanceIds returns the instance IDs of each machine.
	InstanceIds([]names.MachineTag) ([]params.StringResult, error)
}

// LifecycleManager defines an interface used to enable a storage provisioner
// worker to perform lifcycle-related operations on storage entities and
// attachments.
type LifecycleManager interface {
	// Life returns the lifecycle state of the specified entities.
	Life([]names.Tag) ([]params.LifeResult, error)

	// Remove removes the specified entities from state.
	Remove([]names.Tag) ([]params.ErrorResult, error)

	// AttachmentLife returns the lifecycle state of the specified
	// machine/entity attachments.
	AttachmentLife([]params.MachineStorageId) ([]params.LifeResult, error)

	// RemoveAttachments removes the specified machine/entity attachments
	// from state.
	RemoveAttachments([]params.MachineStorageId) ([]params.ErrorResult, error)
}

// EnvironAccessor defines an interface used to enable a storage provisioner
// worker to watch changes to and read environment config, to use when
// provisioning storage.
type EnvironAccessor interface {
	// WatchForEnvironConfigChanges returns a watcher that will be notified
	// whenever the environment config changes in state.
	WatchForEnvironConfigChanges() (apiwatcher.NotifyWatcher, error)

	// EnvironConfig returns the current environment config.
	EnvironConfig() (*config.Config, error)
}

// NewStorageProvisioner returns a Worker which manages
// provisioning (deprovisioning), and attachment (detachment)
// of first-class volumes and filesystems.
//
// Machine-scoped storage workers will be provided with
// a storage directory, while environment-scoped workers
// will not. If the directory path is non-empty, then it
// will be passed to the storage source via its config.
func NewStorageProvisioner(
	scope names.Tag,
	storageDir string,
	v VolumeAccessor,
	f FilesystemAccessor,
	l LifecycleManager,
	e EnvironAccessor,
	m MachineAccessor,
) worker.Worker {
	w := &storageprovisioner{
		scope:       scope,
		storageDir:  storageDir,
		volumes:     v,
		filesystems: f,
		life:        l,
		environ:     e,
		machines:    m,
	}
	go func() {
		defer w.tomb.Done()
		w.tomb.Kill(w.loop())
	}()
	return w
}

type storageprovisioner struct {
	tomb        tomb.Tomb
	scope       names.Tag
	storageDir  string
	volumes     VolumeAccessor
	filesystems FilesystemAccessor
	life        LifecycleManager
	environ     EnvironAccessor
	machines    MachineAccessor
}

// Kill implements Worker.Kill().
func (w *storageprovisioner) Kill() {
	w.tomb.Kill(nil)
}

// Wait implements Worker.Wait().
func (w *storageprovisioner) Wait() error {
	return w.tomb.Wait()
}

func (w *storageprovisioner) loop() error {
	var environConfigChanges <-chan struct{}
	var volumesWatcher apiwatcher.StringsWatcher
	var filesystemsWatcher apiwatcher.StringsWatcher
	var volumesChanges <-chan []string
	var filesystemsChanges <-chan []string
	var volumeAttachmentsWatcher apiwatcher.MachineStorageIdsWatcher
	var filesystemAttachmentsWatcher apiwatcher.MachineStorageIdsWatcher
	var volumeAttachmentsChanges <-chan []params.MachineStorageId
	var filesystemAttachmentsChanges <-chan []params.MachineStorageId
	var machineBlockDevicesWatcher apiwatcher.NotifyWatcher
	var machineBlockDevicesChanges <-chan struct{}
	machineChanges := make(chan names.MachineTag)

	environConfigWatcher, err := w.environ.WatchForEnvironConfigChanges()
	if err != nil {
		return errors.Annotate(err, "watching environ config")
	}
	defer watcher.Stop(environConfigWatcher, &w.tomb)
	environConfigChanges = environConfigWatcher.Changes()

	// Machine-scoped provisioners need to watch block devices, to create
	// volume-backed filesystems.
	if machineTag, ok := w.scope.(names.MachineTag); ok {
		machineBlockDevicesWatcher, err = w.volumes.WatchBlockDevices(machineTag)
		if err != nil {
			return errors.Annotate(err, "watching block devices")
		}
		defer watcher.Stop(machineBlockDevicesWatcher, &w.tomb)
		machineBlockDevicesChanges = machineBlockDevicesWatcher.Changes()
	}

	// The other watchers are started dynamically; stop only if started.
	defer w.maybeStopWatcher(volumesWatcher)
	defer w.maybeStopWatcher(volumeAttachmentsWatcher)
	defer w.maybeStopWatcher(filesystemsWatcher)
	defer w.maybeStopWatcher(filesystemAttachmentsWatcher)

	startWatchers := func() error {
		var err error
		volumesWatcher, err = w.volumes.WatchVolumes()
		if err != nil {
			return errors.Annotate(err, "watching volumes")
		}
		filesystemsWatcher, err = w.filesystems.WatchFilesystems()
		if err != nil {
			return errors.Annotate(err, "watching filesystems")
		}
		volumeAttachmentsWatcher, err = w.volumes.WatchVolumeAttachments()
		if err != nil {
			return errors.Annotate(err, "watching volume attachments")
		}
		filesystemAttachmentsWatcher, err = w.filesystems.WatchFilesystemAttachments()
		if err != nil {
			return errors.Annotate(err, "watching filesystem attachments")
		}
		volumesChanges = volumesWatcher.Changes()
		filesystemsChanges = filesystemsWatcher.Changes()
		volumeAttachmentsChanges = volumeAttachmentsWatcher.Changes()
		filesystemAttachmentsChanges = filesystemAttachmentsWatcher.Changes()
		return nil
	}

	ctx := context{
		scope:                        w.scope,
		storageDir:                   w.storageDir,
		volumeAccessor:               w.volumes,
		filesystemAccessor:           w.filesystems,
		life:                         w.life,
		machineAccessor:              w.machines,
		volumes:                      make(map[names.VolumeTag]storage.Volume),
		volumeAttachments:            make(map[params.MachineStorageId]storage.VolumeAttachment),
		volumeBlockDevices:           make(map[names.VolumeTag]storage.BlockDevice),
		filesystems:                  make(map[names.FilesystemTag]storage.Filesystem),
		filesystemAttachments:        make(map[params.MachineStorageId]storage.FilesystemAttachment),
		machines:                     make(map[names.MachineTag]*machineWatcher),
		machineChanges:               machineChanges,
		pendingVolumes:               make(map[names.VolumeTag]storage.VolumeParams),
		pendingVolumeAttachments:     make(map[params.MachineStorageId]storage.VolumeAttachmentParams),
		pendingVolumeBlockDevices:    make(set.Tags),
		pendingFilesystems:           make(map[names.FilesystemTag]storage.FilesystemParams),
		pendingFilesystemAttachments: make(map[params.MachineStorageId]storage.FilesystemAttachmentParams),
	}
	ctx.managedFilesystemSource = newManagedFilesystemSource(
		ctx.volumeBlockDevices, ctx.filesystems,
	)
	defer func() {
		for _, w := range ctx.machines {
			w.stop()
		}
	}()

	for {
		// Check if any pending operations can be fulfilled.
		if err := processPending(&ctx); err != nil {
			return errors.Trace(err)
		}

		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case _, ok := <-environConfigChanges:
			if !ok {
				return watcher.EnsureErr(environConfigWatcher)
			}
			environConfig, err := w.environ.EnvironConfig()
			if err != nil {
				return errors.Annotate(err, "getting environ config")
			}
			if ctx.environConfig == nil {
				// We've received the initial environ config,
				// so we can begin provisioning storage.
				if err := startWatchers(); err != nil {
					return err
				}
			}
			ctx.environConfig = environConfig
		case changes, ok := <-volumesChanges:
			if !ok {
				return watcher.EnsureErr(volumesWatcher)
			}
			if err := volumesChanged(&ctx, changes); err != nil {
				return errors.Trace(err)
			}
		case changes, ok := <-volumeAttachmentsChanges:
			if !ok {
				return watcher.EnsureErr(volumeAttachmentsWatcher)
			}
			if err := volumeAttachmentsChanged(&ctx, changes); err != nil {
				return errors.Trace(err)
			}
		case changes, ok := <-filesystemsChanges:
			if !ok {
				return watcher.EnsureErr(filesystemsWatcher)
			}
			if err := filesystemsChanged(&ctx, changes); err != nil {
				return errors.Trace(err)
			}
		case changes, ok := <-filesystemAttachmentsChanges:
			if !ok {
				return watcher.EnsureErr(filesystemAttachmentsWatcher)
			}
			if err := filesystemAttachmentsChanged(&ctx, changes); err != nil {
				return errors.Trace(err)
			}
		case _, ok := <-machineBlockDevicesChanges:
			if !ok {
				return watcher.EnsureErr(machineBlockDevicesWatcher)
			}
			if err := machineBlockDevicesChanged(&ctx); err != nil {
				return errors.Trace(err)
			}
		case machineTag := <-machineChanges:
			if err := refreshMachine(&ctx, machineTag); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

// processPending checks if the pending operations' prerequisites have
// been met, and processes them if so.
func processPending(ctx *context) error {
	if err := processPendingVolumes(ctx); err != nil {
		return errors.Annotate(err, "processing pending volumes")
	}
	if err := processPendingVolumeAttachments(ctx); err != nil {
		return errors.Annotate(err, "processing pending volume attachments")
	}
	if err := processPendingVolumeBlockDevices(ctx); err != nil {
		return errors.Annotate(err, "processing pending block devices")
	}
	if err := processPendingFilesystems(ctx); err != nil {
		return errors.Annotate(err, "processing pending filesystems")
	}
	if err := processPendingFilesystemAttachments(ctx); err != nil {
		return errors.Annotate(err, "processing pending filesystem attachments")
	}
	return nil
}

func (p *storageprovisioner) maybeStopWatcher(w watcher.Stopper) {
	if w != nil {
		watcher.Stop(w, &p.tomb)
	}
}

type context struct {
	scope              names.Tag
	environConfig      *config.Config
	storageDir         string
	volumeAccessor     VolumeAccessor
	filesystemAccessor FilesystemAccessor
	life               LifecycleManager
	machineAccessor    MachineAccessor

	// volumes contains information about provisioned volumes.
	volumes map[names.VolumeTag]storage.Volume

	// volumeAttachments contains information about attached volumes.
	volumeAttachments map[params.MachineStorageId]storage.VolumeAttachment

	// volumeBlockDevices contains information about block devices
	// corresponding to volumes attached to the scope-machine. This
	// is only used by the machine-scoped storage provisioner.
	volumeBlockDevices map[names.VolumeTag]storage.BlockDevice

	// filesystems contains information about provisioned filesystems.
	filesystems map[names.FilesystemTag]storage.Filesystem

	// filesystemAttachments contains information about attached filesystems.
	filesystemAttachments map[params.MachineStorageId]storage.FilesystemAttachment

	// machines contains information about machines in the worker's scope.
	machines map[names.MachineTag]*machineWatcher

	// machineChanges is a channel that machine watchers will send to once
	// their machine is known to have been provisioned.
	machineChanges chan<- names.MachineTag

	// pendingVolumes contains parameters for volumes that are yet to be
	// created.
	pendingVolumes map[names.VolumeTag]storage.VolumeParams

	// pendingVolumeAttachments contains parameters for volume attachments
	// that are yet to be created.
	pendingVolumeAttachments map[params.MachineStorageId]storage.VolumeAttachmentParams

	// pendingVolumeBlockDevices contains the tags of volumes about whose
	// block devices we wish to enquire.
	pendingVolumeBlockDevices set.Tags

	// pendingFilesystems contains parameters for filesystems that are
	// yet to be created.
	pendingFilesystems map[names.FilesystemTag]storage.FilesystemParams

	// pendingFilesystemAttachments contains parameters for filesystem attachments
	// that are yet to be created.
	pendingFilesystemAttachments map[params.MachineStorageId]storage.FilesystemAttachmentParams

	// managedFilesystemSource is a storage.FilesystemSource that
	// manages filesystems backed by volumes attached to the host
	// machine.
	managedFilesystemSource storage.FilesystemSource
}
