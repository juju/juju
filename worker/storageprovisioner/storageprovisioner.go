// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"launchpad.net/tomb"

	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.storageprovisioner")

// VolumeAccessor defines an interface used to allow a storage provisioner
// worker to perform volume related operations.
type VolumeAccessor interface {
	// WatchVolumes watches for changes to volumes that this storage
	// provisioner is responsible for.
	WatchVolumes() (apiwatcher.StringsWatcher, error)

	// WatchVolumeAttachments watches for changes to volume attachments
	// that this storage provisioner is responsible for.
	WatchVolumeAttachments() (apiwatcher.MachineStorageIdsWatcher, error)

	// Volumes returns details of volumes with the specified tags.
	Volumes([]names.VolumeTag) ([]params.VolumeResult, error)

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

// LifecycleManager defines an interface used to enable a storage provisioner
// worker to perform lifcycle-related operations on storage entities and
// attachments.
type LifecycleManager interface {
	// Life returns the lifecycle state of the specified entities.
	Life([]names.Tag) ([]params.LifeResult, error)

	// EnsureDead ensures that the specified entities become Dead if
	// they are Alive or Dying.
	EnsureDead([]names.Tag) ([]params.ErrorResult, error)

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
	storageDir string,
	v VolumeAccessor,
	f FilesystemAccessor,
	l LifecycleManager,
	e EnvironAccessor,
) worker.Worker {
	w := &storageprovisioner{
		storageDir:  storageDir,
		volumes:     v,
		filesystems: f,
		life:        l,
		environ:     e,
	}
	go func() {
		defer w.tomb.Done()
		w.tomb.Kill(w.loop())
	}()
	return w
}

type storageprovisioner struct {
	tomb        tomb.Tomb
	storageDir  string
	volumes     VolumeAccessor
	filesystems FilesystemAccessor
	life        LifecycleManager
	environ     EnvironAccessor
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
	var volumesChanges <-chan []string
	var volumeAttachmentsWatcher apiwatcher.MachineStorageIdsWatcher
	var volumeAttachmentsChanges <-chan []params.MachineStorageId

	environConfigWatcher, err := w.environ.WatchForEnvironConfigChanges()
	if err != nil {
		return errors.Annotate(err, "watching environ config")
	}
	defer watcher.Stop(environConfigWatcher, &w.tomb)
	environConfigChanges = environConfigWatcher.Changes()

	// The other watchers are started dynamically; stop only if started.
	defer w.maybeStopWatcher(volumesWatcher)
	defer w.maybeStopWatcher(volumeAttachmentsWatcher)

	startWatchers := func() error {
		var err error
		volumesWatcher, err = w.volumes.WatchVolumes()
		if err != nil {
			return errors.Annotate(err, "watching volumes")
		}
		volumeAttachmentsWatcher, err = w.volumes.WatchVolumeAttachments()
		if err != nil {
			return errors.Annotate(err, "watching volume attachments")
		}
		volumesChanges = volumesWatcher.Changes()
		volumeAttachmentsChanges = volumeAttachmentsWatcher.Changes()
		return nil
	}

	filesystemsWatcher, err := w.filesystems.WatchFilesystems()
	if err != nil {
		return errors.Annotate(err, "watching filesystems")
	}
	defer watcher.Stop(filesystemsWatcher, &w.tomb)
	filesystemsChanges := filesystemsWatcher.Changes()

	filesystemAttachmentsWatcher, err := w.filesystems.WatchFilesystemAttachments()
	if err != nil {
		return errors.Annotate(err, "watching filesystem attachments")
	}
	defer watcher.Stop(filesystemAttachmentsWatcher, &w.tomb)
	filesystemAttachmentsChanges := filesystemAttachmentsWatcher.Changes()

	ctx := context{
		storageDir:  w.storageDir,
		volumes:     w.volumes,
		filesystems: w.filesystems,
		life:        w.life,
	}

	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case _, ok := <-environConfigChanges:
			if !ok {
				return watcher.EnsureErr(volumesWatcher)
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
		}
	}
}

func (p *storageprovisioner) maybeStopWatcher(w watcher.Stopper) {
	if w != nil {
		watcher.Stop(w, &p.tomb)
	}
}

type context struct {
	environConfig *config.Config
	storageDir    string
	volumes       VolumeAccessor
	filesystems   FilesystemAccessor
	life          LifecycleManager
}
