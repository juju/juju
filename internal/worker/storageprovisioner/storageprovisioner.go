// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	"github.com/juju/juju/internal/worker/storageprovisioner/internal/schedule"
	"github.com/juju/juju/rpc/params"
)

var (
	// defaultDependentChangesTimeout is the default timeout for waiting for any
	// dependent changes to occur before proceeding with a storage provisioner.
	// This is a variable so it can be overridden in tests (sigh).
	defaultDependentChangesTimeout = time.Second
)

var newManagedFilesystemSource = provider.NewManagedFilesystemSource

// VolumeAccessor defines an interface used to allow a storage provisioner
// worker to perform volume related operations.
type VolumeAccessor interface {
	// WatchBlockDevices watches for changes to the block devices of the
	// specified machine.
	WatchBlockDevices(context.Context, names.MachineTag) (watcher.NotifyWatcher, error)

	// WatchVolumes watches for changes to volumes that this storage
	// provisioner is responsible for.
	WatchVolumes(ctx context.Context, scope names.Tag) (watcher.StringsWatcher, error)

	// WatchVolumeAttachments watches for changes to volume attachments
	// that this storage provisioner is responsible for.
	WatchVolumeAttachments(ctx context.Context, scope names.Tag) (watcher.MachineStorageIDsWatcher, error)

	// WatchVolumeAttachmentPlans watches for changes to volume attachments
	// destined for this machine. It allows the machine agent to do any extra
	// initialization of the attachment, such as logging into the iSCSI target
	WatchVolumeAttachmentPlans(ctx context.Context, scope names.Tag) (watcher.MachineStorageIDsWatcher, error)

	// Volumes returns details of volumes with the specified tags.
	Volumes(context.Context, []names.VolumeTag) ([]params.VolumeResult, error)

	// VolumeBlockDevices returns details of block devices corresponding to
	// the specified volume attachment IDs.
	VolumeBlockDevices(context.Context, []params.MachineStorageId) ([]params.BlockDeviceResult, error)

	// VolumeAttachments returns details of volume attachments with
	// the specified tags.
	VolumeAttachments(context.Context, []params.MachineStorageId) ([]params.VolumeAttachmentResult, error)

	VolumeAttachmentPlans(context.Context, []params.MachineStorageId) ([]params.VolumeAttachmentPlanResult, error)

	// VolumeParams returns the parameters for creating the volumes
	// with the specified tags.
	VolumeParams(context.Context, []names.VolumeTag) ([]params.VolumeParamsResult, error)

	// RemoveVolumeParams returns the parameters for destroying or
	// releasing the volumes with the specified tags.
	RemoveVolumeParams(context.Context, []names.VolumeTag) ([]params.RemoveVolumeParamsResult, error)

	// VolumeAttachmentParams returns the parameters for creating the
	// volume attachments with the specified tags.
	VolumeAttachmentParams(context.Context, []params.MachineStorageId) ([]params.VolumeAttachmentParamsResult, error)

	// SetVolumeInfo records the details of newly provisioned volumes.
	SetVolumeInfo(context.Context, []params.Volume) ([]params.ErrorResult, error)

	// SetVolumeAttachmentInfo records the details of newly provisioned
	// volume attachments.
	SetVolumeAttachmentInfo(context.Context, []params.VolumeAttachment) ([]params.ErrorResult, error)

	CreateVolumeAttachmentPlans(ctx context.Context, volumeAttachmentPlans []params.VolumeAttachmentPlan) ([]params.ErrorResult, error)
	RemoveVolumeAttachmentPlan(context.Context, []params.MachineStorageId) ([]params.ErrorResult, error)
	SetVolumeAttachmentPlanBlockInfo(ctx context.Context, volumeAttachmentPlans []params.VolumeAttachmentPlan) ([]params.ErrorResult, error)
}

// FilesystemAccessor defines an interface used to allow a storage provisioner
// worker to perform filesystem related operations.
type FilesystemAccessor interface {
	// WatchFilesystems watches for changes to filesystems that this
	// storage provisioner is responsible for.
	WatchFilesystems(ctx context.Context, scope names.Tag) (watcher.StringsWatcher, error)

	// WatchFilesystemAttachments watches for changes to filesystem attachments
	// that this storage provisioner is responsible for.
	WatchFilesystemAttachments(ctx context.Context, scope names.Tag) (watcher.MachineStorageIDsWatcher, error)

	// Filesystems returns details of filesystems with the specified tags.
	Filesystems(context.Context, []names.FilesystemTag) ([]params.FilesystemResult, error)

	// FilesystemAttachments returns details of filesystem attachments with
	// the specified tags.
	FilesystemAttachments(context.Context, []params.MachineStorageId) ([]params.FilesystemAttachmentResult, error)

	// FilesystemParams returns the parameters for creating the filesystems
	// with the specified tags.
	FilesystemParams(context.Context, []names.FilesystemTag) ([]params.FilesystemParamsResult, error)

	// RemoveFilesystemParams returns the parameters for destroying or
	// releasing the filesystems with the specified tags.
	RemoveFilesystemParams(context.Context, []names.FilesystemTag) ([]params.RemoveFilesystemParamsResult, error)

	// FilesystemAttachmentParams returns the parameters for creating the
	// filesystem attachments with the specified tags.
	FilesystemAttachmentParams(context.Context, []params.MachineStorageId) ([]params.FilesystemAttachmentParamsResult, error)

	// SetFilesystemInfo records the details of newly provisioned filesystems.
	SetFilesystemInfo(context.Context, []params.Filesystem) ([]params.ErrorResult, error)

	// SetFilesystemAttachmentInfo records the details of newly provisioned
	// filesystem attachments.
	SetFilesystemAttachmentInfo(context.Context, []params.FilesystemAttachment) ([]params.ErrorResult, error)
}

// MachineAccessor defines an interface used to allow a storage provisioner
// worker to perform machine related operations.
type MachineAccessor interface {
	// WatchMachine watches for changes to the specified machine.
	WatchMachine(context.Context, names.MachineTag) (watcher.NotifyWatcher, error)

	// InstanceIds returns the instance IDs of each machine.
	InstanceIds(context.Context, []names.MachineTag) ([]params.StringResult, error)
}

// LifecycleManager defines an interface used to enable a storage provisioner
// worker to perform lifcycle-related operations on storage entities and
// attachments.
type LifecycleManager interface {
	// Life returns the lifecycle state of the specified entities.
	Life(context.Context, []names.Tag) ([]params.LifeResult, error)

	// Remove removes the specified entities from state.
	Remove(context.Context, []names.Tag) ([]params.ErrorResult, error)

	// AttachmentLife returns the lifecycle state of the specified
	// machine/entity attachments.
	AttachmentLife(context.Context, []params.MachineStorageId) ([]params.LifeResult, error)

	// RemoveAttachments removes the specified machine/entity attachments
	// from state.
	RemoveAttachments(context.Context, []params.MachineStorageId) ([]params.ErrorResult, error)
}

// StatusSetter defines an interface used to set the status of entities.
type StatusSetter interface {
	SetStatus(context.Context, []params.EntityStatusArgs) error
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
		Name: "storage-provisioner",
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
	err := w.catacomb.Wait()
	return err
}

func (w *storageProvisioner) loop() error {
	ctx, cancel := w.scopedContext()
	defer cancel()

	var (
		volumesChanges               watcher.StringsChannel
		filesystemsChanges           watcher.StringsChannel
		volumeAttachmentsChanges     watcher.MachineStorageIDsChannel
		volumeAttachmentPlansChanges watcher.MachineStorageIDsChannel
		filesystemAttachmentsChanges watcher.MachineStorageIDsChannel
		machineBlockDevicesChanges   <-chan struct{}
	)
	machineChanges := make(chan names.MachineTag)

	// Machine-scoped provisioners need to watch block devices, to create
	// volume-backed filesystems.
	if machineTag, ok := w.config.Scope.(names.MachineTag); ok {
		w.config.Logger.Infof(ctx, "starting machine scope storage provisioner")

		machineBlockDevicesWatcher, err := w.config.Volumes.WatchBlockDevices(ctx, machineTag)
		if err != nil {
			return errors.Annotate(err, "watching block devices")
		}
		if err := w.catacomb.Add(machineBlockDevicesWatcher); err != nil {
			return errors.Trace(err)
		}
		machineBlockDevicesChanges = machineBlockDevicesWatcher.Changes()

		volumeAttachmentPlansWatcher, err := w.config.Volumes.WatchVolumeAttachmentPlans(ctx, machineTag)
		if err != nil {
			return errors.Annotate(err, "watching volume attachment plans")
		}
		if err := w.catacomb.Add(volumeAttachmentPlansWatcher); err != nil {
			return errors.Trace(err)
		}

		volumeAttachmentPlansChanges = volumeAttachmentPlansWatcher.Changes()
	} else {
		w.config.Logger.Infof(ctx, "starting model scope storage provisioner")
	}

	deps := dependencies{
		kill:                                 w.catacomb.Kill,
		addWorker:                            w.catacomb.Add,
		config:                               w.config,
		volumes:                              make(map[names.VolumeTag]storage.Volume),
		volumeAttachments:                    make(map[params.MachineStorageId]storage.VolumeAttachment),
		volumeBlockDevices:                   make(map[names.VolumeTag]blockdevice.BlockDevice),
		filesystems:                          make(map[names.FilesystemTag]storage.Filesystem),
		filesystemAttachments:                make(map[params.MachineStorageId]storage.FilesystemAttachment),
		machines:                             make(map[names.MachineTag]*machineWatcher),
		machineChanges:                       machineChanges,
		schedule:                             schedule.NewSchedule(w.config.Clock),
		incompleteVolumeParams:               make(map[names.VolumeTag]storage.VolumeParams),
		incompleteVolumeAttachmentParams:     make(map[params.MachineStorageId]storage.VolumeAttachmentParams),
		incompleteFilesystemParams:           make(map[names.FilesystemTag]storage.FilesystemParams),
		incompleteFilesystemAttachmentParams: make(map[params.MachineStorageId]storage.FilesystemAttachmentParams),
		pendingVolumeBlockDevices:            names.NewSet(),
	}
	deps.managedFilesystemSource = newManagedFilesystemSource(
		deps.volumeBlockDevices, deps.filesystems,
	)
	// Units don't use managed volume backed filesystems.
	if deps.isApplicationKind() {
		deps.managedFilesystemSource = &noopFilesystemSource{}
	}

	// Units don't have unit-scoped volumes - all volumes are
	// associated with the model (namespace).
	if !deps.isApplicationKind() {
		volumesWatcher, err := w.config.Volumes.WatchVolumes(ctx, w.config.Scope)
		if err != nil {
			return errors.Annotate(err, "watching volumes")
		}
		if err := w.catacomb.Add(volumesWatcher); err != nil {
			return errors.Trace(err)
		}
		volumesChanges = volumesWatcher.Changes()
	}

	filesystemsWatcher, err := w.config.Filesystems.WatchFilesystems(ctx, w.config.Scope)
	if err != nil {
		return errors.Annotate(err, "watching filesystems")
	}
	if err := w.catacomb.Add(filesystemsWatcher); err != nil {
		return errors.Trace(err)
	}
	filesystemsChanges = filesystemsWatcher.Changes()

	volumeAttachmentsWatcher, err := w.config.Volumes.WatchVolumeAttachments(ctx, w.config.Scope)
	if err != nil {
		return errors.Annotate(err, "watching volume attachments")
	}
	if err := w.catacomb.Add(volumeAttachmentsWatcher); err != nil {
		return errors.Trace(err)
	}
	volumeAttachmentsChanges = volumeAttachmentsWatcher.Changes()

	filesystemAttachmentsWatcher, err := w.config.Filesystems.WatchFilesystemAttachments(ctx, w.config.Scope)
	if err != nil {
		return errors.Annotate(err, "watching filesystem attachments")
	}
	if err := w.catacomb.Add(filesystemAttachmentsWatcher); err != nil {
		return errors.Trace(err)
	}
	filesystemAttachmentsChanges = filesystemAttachmentsWatcher.Changes()

	for {

		// Check if block devices need to be refreshed.
		if err := processPendingVolumeBlockDevices(ctx, &deps); err != nil {
			return errors.Annotate(err, "processing pending block devices")
		}

		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case changes, ok := <-volumesChanges:
			if !ok {
				return errors.New("volumes watcher closed")
			}
			w.config.Logger.Infof(ctx, "volume changes: %v", changes)
			if err := volumesChanged(ctx, &deps, changes); err != nil {
				return errors.Trace(err)
			}
		case changes, ok := <-volumeAttachmentsChanges:
			if !ok {
				return errors.New("volume attachments watcher closed")
			}
			w.config.Logger.Infof(ctx, "volume attachment changes: %v", changes)
			// Process volume changes before volume attachments changes.
			// This is because volume attachments are dependent on
			// volumes, and reveals itself during a reboot of a machine. All
			// the watcher changes come at once, but the order of the select
			// case statements is not guaranteed.
			if err := w.processDependentChanges(ctx, &deps, volumesChanges, volumesChanged); err != nil {
				return errors.Trace(err)
			}
			if err := volumeAttachmentsChanged(ctx, &deps, changes); err != nil {
				return errors.Trace(err)
			}
		case changes, ok := <-volumeAttachmentPlansChanges:
			if !ok {
				return errors.New("volume attachment plans watcher closed")
			}
			w.config.Logger.Infof(ctx, "volume attachment plan changes: %v", changes)
			if err := volumeAttachmentPlansChanged(ctx, &deps, changes); err != nil {
				return errors.Trace(err)
			}
		case changes, ok := <-filesystemsChanges:
			if !ok {
				return errors.New("filesystems watcher closed")
			}
			w.config.Logger.Infof(ctx, "filesystem changes: %v", changes)
			if err := filesystemsChanged(ctx, &deps, changes); err != nil {
				return errors.Trace(err)
			}
		case changes, ok := <-filesystemAttachmentsChanges:
			if !ok {
				return errors.New("filesystem attachments watcher closed")
			}
			w.config.Logger.Infof(ctx, "filesystem attachment changes: %v", changes)
			// Process filesystem changes before filesystem attachments changes.
			// This is because filesystem attachments are dependent on
			// filesystems, and reveals itself during a reboot of a machine. All
			// the watcher changes come at once, but the order of the select
			// case statements is not guaranteed.
			if err := w.processDependentChanges(ctx, &deps, filesystemsChanges, filesystemsChanged); err != nil {
				return errors.Trace(err)
			}
			if err := filesystemAttachmentsChanged(ctx, &deps, changes); err != nil {
				return errors.Trace(err)
			}
		case _, ok := <-machineBlockDevicesChanges:
			if !ok {
				return errors.New("machine block devices watcher closed")
			}
			w.config.Logger.Infof(ctx, "block devices changed")
			if err := machineBlockDevicesChanged(ctx, &deps); err != nil {
				return errors.Trace(err)
			}
		case machineTag := <-machineChanges:
			w.config.Logger.Infof(ctx, "machine changed: %v", machineTag)
			if err := refreshMachine(ctx, &deps, machineTag); err != nil {
				return errors.Trace(err)
			}
		case <-deps.schedule.Next():
			// Ready to pick something(s) off the pending queue.
			if err := processSchedule(ctx, &deps); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

// processDependentChanges processes changes from a watcher strings channel. If
// there are any changes, it calls the given function, repeating until there are
// no more changes.
// If there are no changes, it returns with no error.
func (w *storageProvisioner) processDependentChanges(ctx context.Context, deps *dependencies, source watcher.StringsChannel, fn func(context.Context, *dependencies, []string) error) error {
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case changes, ok := <-source:
			if !ok {
				return errors.New("watcher closed")
			}
			if err := fn(ctx, deps, changes); err != nil {
				return errors.Trace(err)
			}
		case <-time.After(defaultDependentChangesTimeout):
			// Nothing to do, we've waited long enough.
			return nil
		}
	}
}

func (w *storageProvisioner) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}

// processSchedule executes scheduled operations.
func processSchedule(ctx context.Context, deps *dependencies) error {
	ready := deps.schedule.Ready(deps.config.Clock.Now())
	createVolumeOps := make(map[names.VolumeTag]*createVolumeOp)
	removeVolumeOps := make(map[names.VolumeTag]*removeVolumeOp)
	attachVolumeOps := make(map[params.MachineStorageId]*attachVolumeOp)
	detachVolumeOps := make(map[params.MachineStorageId]*detachVolumeOp)
	createFilesystemOps := make(map[names.FilesystemTag]*createFilesystemOp)
	removeFilesystemOps := make(map[names.FilesystemTag]*removeFilesystemOp)
	attachFilesystemOps := make(map[params.MachineStorageId]*attachFilesystemOp)
	detachFilesystemOps := make(map[params.MachineStorageId]*detachFilesystemOp)
	for _, item := range ready {
		op := item.(scheduleOp)
		key := op.key()
		switch op := op.(type) {
		case *createVolumeOp:
			createVolumeOps[key.(names.VolumeTag)] = op
		case *removeVolumeOp:
			removeVolumeOps[key.(names.VolumeTag)] = op
		case *attachVolumeOp:
			attachVolumeOps[key.(params.MachineStorageId)] = op
		case *detachVolumeOp:
			detachVolumeOps[key.(params.MachineStorageId)] = op
		case *createFilesystemOp:
			createFilesystemOps[key.(names.FilesystemTag)] = op
		case *removeFilesystemOp:
			removeFilesystemOps[key.(names.FilesystemTag)] = op
		case *attachFilesystemOp:
			attachFilesystemOps[key.(params.MachineStorageId)] = op
		case *detachFilesystemOp:
			detachFilesystemOps[key.(params.MachineStorageId)] = op
		}
	}
	if len(removeVolumeOps) > 0 {
		if err := removeVolumes(ctx, deps, removeVolumeOps); err != nil {
			return errors.Annotate(err, "removing volumes")
		}
	}
	if len(createVolumeOps) > 0 {
		if err := createVolumes(ctx, deps, createVolumeOps); err != nil {
			return errors.Annotate(err, "creating volumes")
		}
	}
	if len(detachVolumeOps) > 0 {
		if err := detachVolumes(ctx, deps, detachVolumeOps); err != nil {
			return errors.Annotate(err, "detaching volumes")
		}
	}
	if len(attachVolumeOps) > 0 {
		if err := attachVolumes(ctx, deps, attachVolumeOps); err != nil {
			return errors.Annotate(err, "attaching volumes")
		}
	}
	if len(removeFilesystemOps) > 0 {
		if err := removeFilesystems(ctx, deps, removeFilesystemOps); err != nil {
			return errors.Annotate(err, "removing filesystems")
		}
	}
	if len(createFilesystemOps) > 0 {
		if err := createFilesystems(ctx, deps, createFilesystemOps); err != nil {
			return errors.Annotate(err, "creating filesystems")
		}
	}
	if len(detachFilesystemOps) > 0 {
		if err := detachFilesystems(ctx, deps, detachFilesystemOps); err != nil {
			return errors.Annotate(err, "detaching filesystems")
		}
	}
	if len(attachFilesystemOps) > 0 {
		if err := attachFilesystems(ctx, deps, attachFilesystemOps); err != nil {
			return errors.Annotate(err, "attaching filesystems")
		}
	}
	return nil
}

type dependencies struct {
	kill      func(error)
	addWorker func(worker.Worker) error
	config    Config

	// volumes contains information about provisioned volumes.
	volumes map[names.VolumeTag]storage.Volume

	// volumeAttachments contains information about attached volumes.
	volumeAttachments map[params.MachineStorageId]storage.VolumeAttachment

	// volumeBlockDevices contains information about block devices
	// corresponding to volumes attached to the scope-machine. This
	// is only used by the machine-scoped storage provisioner.
	volumeBlockDevices map[names.VolumeTag]blockdevice.BlockDevice

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
	pendingVolumeBlockDevices names.Set

	// managedFilesystemSource is a storage.FilesystemSource that
	// manages filesystems backed by volumes attached to the host
	// machine.
	managedFilesystemSource storage.FilesystemSource
}

func (c *dependencies) isApplicationKind() bool {
	return c.config.Scope.Kind() == names.ApplicationTagKind
}
