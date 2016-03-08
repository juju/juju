// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package storageprovisioner provides a worker that manages the provisioning
// and deprovisioning of storage volumes and filesystems, and attaching them
// to and detaching them from machines.
//
// A storage provisioner worker is run at each model manager, which
// manages model-scoped storage such as virtual disk services of the
// cloud provider. In addition to this, each machine agent runs a machine-
// storage provisioner worker that manages storage scoped to that machine,
// such as loop devices, temporary filesystems (tmpfs), and rootfs.
//
// The storage provisioner worker is comprised of the following major
// components:
//  - a set of watchers for provisioning and attachment events
//  - a schedule of pending operations
//  - event-handling code fed by the watcher, that identifies
//    interesting changes (unprovisioned -> provisioned, etc.),
//    ensures prerequisites are met (e.g. volume and machine are both
//    provisioned before attachment is attempted), and populates
//    operations into the schedule
//  - operation execution code fed by the schedule, that groups
//    operations to make bulk calls to storage providers; updates
//    status; and reschedules operations upon failure
//
package storageprovisioner

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/set"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/catacomb"
	"github.com/juju/juju/worker/storageprovisioner/internal/schedule"
)

var logger = loggo.GetLogger("juju.worker.storageprovisioner")

var newManagedFilesystemSource = provider.NewManagedFilesystemSource

// VolumeAccessor defines an interface used to allow a storage provisioner
// worker to perform volume related operations.
type VolumeAccessor interface {
	// WatchBlockDevices watches for changes to the block devices of the
	// specified machine.
	WatchBlockDevices(names.MachineTag) (watcher.NotifyWatcher, error)

	// WatchVolumes watches for changes to volumes that this storage
	// provisioner is responsible for.
	WatchVolumes() (watcher.StringsWatcher, error)

	// WatchVolumeAttachments watches for changes to volume attachments
	// that this storage provisioner is responsible for.
	WatchVolumeAttachments() (watcher.MachineStorageIdsWatcher, error)

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
	WatchFilesystems() (watcher.StringsWatcher, error)

	// WatchFilesystemAttachments watches for changes to filesystem attachments
	// that this storage provisioner is responsible for.
	WatchFilesystemAttachments() (watcher.MachineStorageIdsWatcher, error)

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
	WatchMachine(names.MachineTag) (watcher.NotifyWatcher, error)

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

// StatusSetter defines an interface used to set the status of entities.
type StatusSetter interface {
	SetStatus([]params.EntityStatusArgs) error
}

// ModelAccessor defines an interface used to enable a storage provisioner
// worker to watch changes to and read model config, to use when
// provisioning storage.
type ModelAccessor interface {
	// WatchForModelConfigChanges returns a watcher that will be notified
	// whenever the model config changes in state.
	WatchForModelConfigChanges() (watcher.NotifyWatcher, error)

	// ModelConfig returns the current model config.
	ModelConfig() (*config.Config, error)
}

// NewStorageProvisioner returns a Worker which manages
// provisioning (deprovisioning), and attachment (detachment)
// of first-class volumes and filesystems.
//
// Machine-scoped storage workers will be provided with
// a storage directory, while model-scoped workers
// will not. If the directory path is non-empty, then it
// will be passed to the storage source via its config.
var NewStorageProvisioner = func(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	w := &storageProvisioner{
		config: config,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

type storageProvisioner struct {
	catacomb catacomb.Catacomb
	config   Config
}

// Kill implements Worker.Kill().
func (w *storageProvisioner) Kill() {
	w.catacomb.Kill(nil)
}

// Wait implements Worker.Wait().
func (w *storageProvisioner) Wait() error {
	return w.catacomb.Wait()
}

func (w *storageProvisioner) loop() error {
	var (
		volumesChanges               watcher.StringsChannel
		filesystemsChanges           watcher.StringsChannel
		volumeAttachmentsChanges     watcher.MachineStorageIdsChannel
		filesystemAttachmentsChanges watcher.MachineStorageIdsChannel
		machineBlockDevicesChanges   <-chan struct{}
	)
	machineChanges := make(chan names.MachineTag)

	modelConfigWatcher, err := w.config.Environ.WatchForModelConfigChanges()
	if err != nil {
		return errors.Annotate(err, "watching model config")
	}
	if err := w.catacomb.Add(modelConfigWatcher); err != nil {
		return errors.Trace(err)
	}

	// Machine-scoped provisioners need to watch block devices, to create
	// volume-backed filesystems.
	if machineTag, ok := w.config.Scope.(names.MachineTag); ok {
		machineBlockDevicesWatcher, err := w.config.Volumes.WatchBlockDevices(machineTag)
		if err != nil {
			return errors.Annotate(err, "watching block devices")
		}
		if err := w.catacomb.Add(machineBlockDevicesWatcher); err != nil {
			return errors.Trace(err)
		}
		machineBlockDevicesChanges = machineBlockDevicesWatcher.Changes()
	}

	startWatchers := func() error {
		volumesWatcher, err := w.config.Volumes.WatchVolumes()
		if err != nil {
			return errors.Annotate(err, "watching volumes")
		}
		if err := w.catacomb.Add(volumesWatcher); err != nil {
			return errors.Trace(err)
		}
		volumesChanges = volumesWatcher.Changes()

		filesystemsWatcher, err := w.config.Filesystems.WatchFilesystems()
		if err != nil {
			return errors.Annotate(err, "watching filesystems")
		}
		if err := w.catacomb.Add(filesystemsWatcher); err != nil {
			return errors.Trace(err)
		}
		filesystemsChanges = filesystemsWatcher.Changes()

		volumeAttachmentsWatcher, err := w.config.Volumes.WatchVolumeAttachments()
		if err != nil {
			return errors.Annotate(err, "watching volume attachments")
		}
		if err := w.catacomb.Add(volumeAttachmentsWatcher); err != nil {
			return errors.Trace(err)
		}
		volumeAttachmentsChanges = volumeAttachmentsWatcher.Changes()

		filesystemAttachmentsWatcher, err := w.config.Filesystems.WatchFilesystemAttachments()
		if err != nil {
			return errors.Annotate(err, "watching filesystem attachments")
		}
		if err := w.catacomb.Add(filesystemAttachmentsWatcher); err != nil {
			return errors.Trace(err)
		}
		filesystemAttachmentsChanges = filesystemAttachmentsWatcher.Changes()
		return nil
	}

	ctx := context{
		kill:                                 w.catacomb.Kill,
		addWorker:                            w.catacomb.Add,
		config:                               w.config,
		volumes:                              make(map[names.VolumeTag]storage.Volume),
		volumeAttachments:                    make(map[params.MachineStorageId]storage.VolumeAttachment),
		volumeBlockDevices:                   make(map[names.VolumeTag]storage.BlockDevice),
		filesystems:                          make(map[names.FilesystemTag]storage.Filesystem),
		filesystemAttachments:                make(map[params.MachineStorageId]storage.FilesystemAttachment),
		machines:                             make(map[names.MachineTag]*machineWatcher),
		machineChanges:                       machineChanges,
		schedule:                             schedule.NewSchedule(w.config.Clock),
		incompleteVolumeParams:               make(map[names.VolumeTag]storage.VolumeParams),
		incompleteVolumeAttachmentParams:     make(map[params.MachineStorageId]storage.VolumeAttachmentParams),
		incompleteFilesystemParams:           make(map[names.FilesystemTag]storage.FilesystemParams),
		incompleteFilesystemAttachmentParams: make(map[params.MachineStorageId]storage.FilesystemAttachmentParams),
		pendingVolumeBlockDevices:            make(set.Tags),
	}
	ctx.managedFilesystemSource = newManagedFilesystemSource(
		ctx.volumeBlockDevices, ctx.filesystems,
	)
	for {

		// Check if block devices need to be refreshed.
		if err := processPendingVolumeBlockDevices(&ctx); err != nil {
			return errors.Annotate(err, "processing pending block devices")
		}

		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case _, ok := <-modelConfigWatcher.Changes():
			if !ok {
				return errors.New("environ config watcher closed")
			}
			modelConfig, err := w.config.Environ.ModelConfig()
			if err != nil {
				return errors.Annotate(err, "getting model config")
			}
			if ctx.modelConfig == nil {
				// We've received the initial model config,
				// so we can begin provisioning storage.
				if err := startWatchers(); err != nil {
					return err
				}
			}
			ctx.modelConfig = modelConfig
		case changes, ok := <-volumesChanges:
			if !ok {
				return errors.New("volumes watcher closed")
			}
			if err := volumesChanged(&ctx, changes); err != nil {
				return errors.Trace(err)
			}
		case changes, ok := <-volumeAttachmentsChanges:
			if !ok {
				return errors.New("volume attachments watcher closed")
			}
			if err := volumeAttachmentsChanged(&ctx, changes); err != nil {
				return errors.Trace(err)
			}
		case changes, ok := <-filesystemsChanges:
			if !ok {
				return errors.New("filesystems watcher closed")
			}
			if err := filesystemsChanged(&ctx, changes); err != nil {
				return errors.Trace(err)
			}
		case changes, ok := <-filesystemAttachmentsChanges:
			if !ok {
				return errors.New("filesystem attachments watcher closed")
			}
			if err := filesystemAttachmentsChanged(&ctx, changes); err != nil {
				return errors.Trace(err)
			}
		case _, ok := <-machineBlockDevicesChanges:
			if !ok {
				return errors.New("machine block devices watcher closed")
			}
			if err := machineBlockDevicesChanged(&ctx); err != nil {
				return errors.Trace(err)
			}
		case machineTag := <-machineChanges:
			if err := refreshMachine(&ctx, machineTag); err != nil {
				return errors.Trace(err)
			}
		case <-ctx.schedule.Next():
			// Ready to pick something(s) off the pending queue.
			if err := processSchedule(&ctx); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

// processSchedule executes scheduled operations.
func processSchedule(ctx *context) error {
	ready := ctx.schedule.Ready(ctx.config.Clock.Now())
	createVolumeOps := make(map[names.VolumeTag]*createVolumeOp)
	destroyVolumeOps := make(map[names.VolumeTag]*destroyVolumeOp)
	attachVolumeOps := make(map[params.MachineStorageId]*attachVolumeOp)
	detachVolumeOps := make(map[params.MachineStorageId]*detachVolumeOp)
	createFilesystemOps := make(map[names.FilesystemTag]*createFilesystemOp)
	destroyFilesystemOps := make(map[names.FilesystemTag]*destroyFilesystemOp)
	attachFilesystemOps := make(map[params.MachineStorageId]*attachFilesystemOp)
	detachFilesystemOps := make(map[params.MachineStorageId]*detachFilesystemOp)
	for _, item := range ready {
		op := item.(scheduleOp)
		key := op.key()
		switch op := op.(type) {
		case *createVolumeOp:
			createVolumeOps[key.(names.VolumeTag)] = op
		case *destroyVolumeOp:
			destroyVolumeOps[key.(names.VolumeTag)] = op
		case *attachVolumeOp:
			attachVolumeOps[key.(params.MachineStorageId)] = op
		case *detachVolumeOp:
			detachVolumeOps[key.(params.MachineStorageId)] = op
		case *createFilesystemOp:
			createFilesystemOps[key.(names.FilesystemTag)] = op
		case *destroyFilesystemOp:
			destroyFilesystemOps[key.(names.FilesystemTag)] = op
		case *attachFilesystemOp:
			attachFilesystemOps[key.(params.MachineStorageId)] = op
		case *detachFilesystemOp:
			detachFilesystemOps[key.(params.MachineStorageId)] = op
		}
	}
	if len(destroyVolumeOps) > 0 {
		if err := destroyVolumes(ctx, destroyVolumeOps); err != nil {
			return errors.Annotate(err, "destroying volumes")
		}
	}
	if len(createVolumeOps) > 0 {
		if err := createVolumes(ctx, createVolumeOps); err != nil {
			return errors.Annotate(err, "creating volumes")
		}
	}
	if len(detachVolumeOps) > 0 {
		if err := detachVolumes(ctx, detachVolumeOps); err != nil {
			return errors.Annotate(err, "detaching volumes")
		}
	}
	if len(attachVolumeOps) > 0 {
		if err := attachVolumes(ctx, attachVolumeOps); err != nil {
			return errors.Annotate(err, "attaching volumes")
		}
	}
	if len(destroyFilesystemOps) > 0 {
		if err := destroyFilesystems(ctx, destroyFilesystemOps); err != nil {
			return errors.Annotate(err, "destroying filesystems")
		}
	}
	if len(createFilesystemOps) > 0 {
		if err := createFilesystems(ctx, createFilesystemOps); err != nil {
			return errors.Annotate(err, "creating filesystems")
		}
	}
	if len(detachFilesystemOps) > 0 {
		if err := detachFilesystems(ctx, detachFilesystemOps); err != nil {
			return errors.Annotate(err, "detaching filesystems")
		}
	}
	if len(attachFilesystemOps) > 0 {
		if err := attachFilesystems(ctx, attachFilesystemOps); err != nil {
			return errors.Annotate(err, "attaching filesystems")
		}
	}
	return nil
}

type context struct {
	kill        func(error)
	addWorker   func(worker.Worker) error
	config      Config
	modelConfig *config.Config

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

	// schedule is the schedule of storage operations.
	schedule *schedule.Schedule

	// incompleteVolumeParams contains incomplete parameters for volumes.
	//
	// Volume parameters are incomplete when they lack information about
	// the initial attachment. Once the initial attachment information is
	// available, the parameters are removed from this map and a volume
	// creation operation is scheduled.
	incompleteVolumeParams map[names.VolumeTag]storage.VolumeParams

	// incompleteVolumeAttachmentParams contains incomplete parameters
	// for volume attachments
	//
	// Volume attachment parameters are incomplete when they lack
	// information about the associated volume or machine. Once this
	// information is available, the parameters are removed from this
	// map and a volume attachment operation is scheduled.
	incompleteVolumeAttachmentParams map[params.MachineStorageId]storage.VolumeAttachmentParams

	// incompleteFilesystemParams contains incomplete parameters for
	// filesystems.
	//
	// Filesystem parameters are incomplete when they lack information
	// about the initial attachment. Once the initial attachment
	// information is available, the parameters are removed from this
	// map and a filesystem creation operation is scheduled.
	incompleteFilesystemParams map[names.FilesystemTag]storage.FilesystemParams

	// incompleteFilesystemAttachmentParams contains incomplete parameters
	// for filesystem attachments
	//
	// Filesystem attachment parameters are incomplete when they lack
	// information about the associated filesystem or machine. Once this
	// information is available, the parameters are removed from this
	// map and a filesystem attachment operation is scheduled.
	incompleteFilesystemAttachmentParams map[params.MachineStorageId]storage.FilesystemAttachmentParams

	// pendingVolumeBlockDevices contains the tags of volumes about whose
	// block devices we wish to enquire.
	pendingVolumeBlockDevices set.Tags

	// managedFilesystemSource is a storage.FilesystemSource that
	// manages filesystems backed by volumes attached to the host
	// machine.
	managedFilesystemSource storage.FilesystemSource
}
