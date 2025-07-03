// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/worker/storageprovisioner/internal/schedule"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
)

var (
	// defaultDependentChangesTimeout is the default timeout for waiting for any
	// dependent changes to occur before proceeding with a storage provisioner.
	// This is a variable so it can be overridden in tests (sigh).
	defaultDependentChangesTimeout = time.Second
)

// logger is here to stop the desire of creating a package level logger.
// Don't do this, instead use the one passed as manifold config.
type logger interface{}

var _ logger = struct{}{}

var newManagedFilesystemSource = provider.NewManagedFilesystemSource

// VolumeAccessor defines an interface used to allow a storage provisioner
// worker to perform volume related operations.
type VolumeAccessor interface {
	// WatchBlockDevices watches for changes to the block devices of the
	// specified machine.
	WatchBlockDevices(names.MachineTag) (watcher.NotifyWatcher, error)

	// WatchVolumes watches for changes to volumes that this storage
	// provisioner is responsible for.
	WatchVolumes(scope names.Tag) (watcher.StringsWatcher, error)

	// WatchVolumeAttachments watches for changes to volume attachments
	// that this storage provisioner is responsible for.
	WatchVolumeAttachments(scope names.Tag) (watcher.MachineStorageIdsWatcher, error)

	// WatchVolumeAttachmentPlans watches for changes to volume attachments
	// destined for this machine. It allows the machine agent to do any extra
	// initialization of the attachment, such as logging into the iSCSI target
	WatchVolumeAttachmentPlans(scope names.Tag) (watcher.MachineStorageIdsWatcher, error)

	// Volumes returns details of volumes with the specified tags.
	Volumes([]names.VolumeTag) ([]params.VolumeResult, error)

	// VolumeBlockDevices returns details of block devices corresponding to
	// the specified volume attachment IDs.
	VolumeBlockDevices([]params.MachineStorageId) ([]params.BlockDeviceResult, error)

	// VolumeAttachments returns details of volume attachments with
	// the specified tags.
	VolumeAttachments([]params.MachineStorageId) ([]params.VolumeAttachmentResult, error)

	VolumeAttachmentPlans([]params.MachineStorageId) ([]params.VolumeAttachmentPlanResult, error)

	// VolumeParams returns the parameters for creating the volumes
	// with the specified tags.
	VolumeParams([]names.VolumeTag) ([]params.VolumeParamsResult, error)

	// RemoveVolumeParams returns the parameters for destroying or
	// releasing the volumes with the specified tags.
	RemoveVolumeParams([]names.VolumeTag) ([]params.RemoveVolumeParamsResult, error)

	// VolumeAttachmentParams returns the parameters for creating the
	// volume attachments with the specified tags.
	VolumeAttachmentParams([]params.MachineStorageId) ([]params.VolumeAttachmentParamsResult, error)

	// SetVolumeInfo records the details of newly provisioned volumes.
	SetVolumeInfo([]params.Volume) ([]params.ErrorResult, error)

	// SetVolumeAttachmentInfo records the details of newly provisioned
	// volume attachments.
	SetVolumeAttachmentInfo([]params.VolumeAttachment) ([]params.ErrorResult, error)

	CreateVolumeAttachmentPlans(volumeAttachmentPlans []params.VolumeAttachmentPlan) ([]params.ErrorResult, error)
	RemoveVolumeAttachmentPlan([]params.MachineStorageId) ([]params.ErrorResult, error)
	SetVolumeAttachmentPlanBlockInfo(volumeAttachmentPlans []params.VolumeAttachmentPlan) ([]params.ErrorResult, error)
}

// FilesystemAccessor defines an interface used to allow a storage provisioner
// worker to perform filesystem related operations.
type FilesystemAccessor interface {
	// WatchFilesystems watches for changes to filesystems that this
	// storage provisioner is responsible for.
	WatchFilesystems(scope names.Tag) (watcher.StringsWatcher, error)

	// WatchFilesystemAttachments watches for changes to filesystem attachments
	// that this storage provisioner is responsible for.
	WatchFilesystemAttachments(scope names.Tag) (watcher.MachineStorageIdsWatcher, error)

	// Filesystems returns details of filesystems with the specified tags.
	Filesystems([]names.FilesystemTag) ([]params.FilesystemResult, error)

	// FilesystemAttachments returns details of filesystem attachments with
	// the specified tags.
	FilesystemAttachments([]params.MachineStorageId) ([]params.FilesystemAttachmentResult, error)

	// FilesystemParams returns the parameters for creating the filesystems
	// with the specified tags.
	FilesystemParams([]names.FilesystemTag) ([]params.FilesystemParamsResult, error)

	// RemoveFilesystemParams returns the parameters for destroying or
	// releasing the filesystems with the specified tags.
	RemoveFilesystemParams([]names.FilesystemTag) ([]params.RemoveFilesystemParamsResult, error)

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

// NewStorageProvisioner returns a Worker which manages
// provisioning (deprovisioning), and attachment (detachment)
// of first-class volumes and filesystems.
//
// Machine-scoped storage workers will be provided with
// a storage directory, while model-scoped workers
// will not. If the directory path is non-empty, then it
// will be passed to the storage source via its config.
var NewStorageProvisioner = func(config Config) (worker.Worker, error) {
	config.Logger.Debugf("alvin new storage provisioner config: %+v", config)
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
	err := w.catacomb.Wait()
	return err
}

func (w *storageProvisioner) loop() error {
	w.config.Logger.Debugf("alvin storage provisioner loop start")
	var (
		volumesChanges               watcher.StringsChannel
		filesystemsChanges           watcher.StringsChannel
		volumeAttachmentsChanges     watcher.MachineStorageIdsChannel
		volumeAttachmentPlansChanges watcher.MachineStorageIdsChannel
		filesystemAttachmentsChanges watcher.MachineStorageIdsChannel
		machineBlockDevicesChanges   <-chan struct{}
	)
	machineChanges := make(chan names.MachineTag)

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

		volumeAttachmentPlansWatcher, err := w.config.Volumes.WatchVolumeAttachmentPlans(machineTag)
		if err != nil {
			return errors.Annotate(err, "watching volume attachment plans")
		}
		if err := w.catacomb.Add(volumeAttachmentPlansWatcher); err != nil {
			return errors.Trace(err)
		}

		volumeAttachmentPlansChanges = volumeAttachmentPlansWatcher.Changes()
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
		pendingVolumeBlockDevices:            names.NewSet(),
	}
	ctx.managedFilesystemSource = newManagedFilesystemSource(
		ctx.volumeBlockDevices, ctx.filesystems,
	)
	// Units don't use managed volume backed filesystems.
	if ctx.isApplicationKind() {
		ctx.managedFilesystemSource = &noopFilesystemSource{}
	}

	// Units don't have unit-scoped volumes - all volumes are
	// associated with the model (namespace).
	if !ctx.isApplicationKind() {
		volumesWatcher, err := w.config.Volumes.WatchVolumes(w.config.Scope)
		if err != nil {
			return errors.Annotate(err, "watching volumes")
		}
		if err := w.catacomb.Add(volumesWatcher); err != nil {
			return errors.Trace(err)
		}
		volumesChanges = volumesWatcher.Changes()
	}

	filesystemsWatcher, err := w.config.Filesystems.WatchFilesystems(w.config.Scope)
	if err != nil {
		return errors.Annotate(err, "watching filesystems")
	}
	if err := w.catacomb.Add(filesystemsWatcher); err != nil {
		return errors.Trace(err)
	}
	filesystemsChanges = filesystemsWatcher.Changes()

	volumeAttachmentsWatcher, err := w.config.Volumes.WatchVolumeAttachments(w.config.Scope)
	if err != nil {
		return errors.Annotate(err, "watching volume attachments")
	}
	if err := w.catacomb.Add(volumeAttachmentsWatcher); err != nil {
		return errors.Trace(err)
	}
	volumeAttachmentsChanges = volumeAttachmentsWatcher.Changes()

	filesystemAttachmentsWatcher, err := w.config.Filesystems.WatchFilesystemAttachments(w.config.Scope)
	if err != nil {
		return errors.Annotate(err, "watching filesystem attachments")
	}
	if err := w.catacomb.Add(filesystemAttachmentsWatcher); err != nil {
		return errors.Trace(err)
	}
	filesystemAttachmentsChanges = filesystemAttachmentsWatcher.Changes()

	for {

		// Check if block devices need to be refreshed.
		if err := processPendingVolumeBlockDevices(&ctx); err != nil {
			return errors.Annotate(err, "processing pending block devices")
		}

		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
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
			// Process volume changes before volume attachments changes.
			// This is because volume attachments are dependent on
			// volumes, and reveals itself during a reboot of a machine. All
			// the watcher changes come at once, but the order of the select
			// case statements is not guaranteed.
			if err := w.processDependentChanges(&ctx, volumesChanges, volumesChanged); err != nil {
				return errors.Trace(err)
			}
			if err := volumeAttachmentsChanged(&ctx, changes); err != nil {
				return errors.Trace(err)
			}
		case changes, ok := <-volumeAttachmentPlansChanges:
			if !ok {
				return errors.New("volume attachment plans watcher closed")
			}
			if err := volumeAttachmentPlansChanged(&ctx, changes); err != nil {
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
			// Process filesystem changes before filesystem attachments changes.
			// This is because filesystem attachments are dependent on
			// filesystems, and reveals itself during a reboot of a machine. All
			// the watcher changes come at once, but the order of the select
			// case statements is not guaranteed.
			if err := w.processDependentChanges(&ctx, filesystemsChanges, filesystemsChanged); err != nil {
				return errors.Trace(err)
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

// processDependentChanges processes changes from a watcher strings channel. If
// there are any changes, it calls the given function, repeating until there are
// no more changes.
// If there are no changes, it returns with no error.
func (w *storageProvisioner) processDependentChanges(ctx *context, source watcher.StringsChannel, fn func(*context, []string) error) error {
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case changes, ok := <-source:
			if !ok {
				return errors.New("watcher closed")
			}
			if err := fn(ctx, changes); err != nil {
				return errors.Trace(err)
			}
		case <-time.After(defaultDependentChangesTimeout):
			// Nothing to do, we've waited long enough.
			return nil
		}
	}
}

// processSchedule executes scheduled operations.
func processSchedule(ctx *context) error {
	ready := ctx.schedule.Ready(ctx.config.Clock.Now())
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

		if fs, ok := op.(*createFilesystemOp); ok {
			ctx.config.Logger.Debugf("alvin processSchedule createFilesystemOp: %v", fs)
		}
		if fs, ok := op.(*attachFilesystemOp); ok {
			ctx.config.Logger.Debugf("alvin processSchedule attachFilesystemOp: %v", fs)
		}
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
		if err := removeVolumes(ctx, removeVolumeOps); err != nil {
			return errors.Annotate(err, "removing volumes")
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
	if len(removeFilesystemOps) > 0 {
		if err := removeFilesystems(ctx, removeFilesystemOps); err != nil {
			return errors.Annotate(err, "removing filesystems")
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
	pendingVolumeBlockDevices names.Set

	// managedFilesystemSource is a storage.FilesystemSource that
	// manages filesystems backed by volumes attached to the host
	// machine.
	managedFilesystemSource storage.FilesystemSource
}

func (c *context) isApplicationKind() bool {
	return c.config.Scope.Kind() == names.ApplicationTagKind
}
