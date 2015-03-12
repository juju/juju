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
	// WatchVolumes watches for changes to volumes scoped to the
	// entity with the tag passed to NewState.
	WatchVolumes() (apiwatcher.StringsWatcher, error)

	// Volumes returns details of volumes with the specified tags.
	Volumes([]names.VolumeTag) ([]params.VolumeResult, error)

	// VolumeParams returns the parameters for creating the volumes
	// with the specified tags.
	VolumeParams([]names.VolumeTag) ([]params.VolumeParamsResult, error)

	// SetVolumeInfo records the details of newly provisioned volumes.
	SetVolumeInfo([]params.Volume) (params.ErrorResults, error)
}

// LifecycleManager defines an interface used to allow a storage provisioner
// worker to perform volume lifecycle operations.
type LifecycleManager interface {
	// Life requests the life cycle of the entities with the specified tags.
	Life([]names.Tag) ([]params.LifeResult, error)

	// EnsureDead progresses the entities with the specified tags to the Dead
	// life cycle state, if they are Alive or Dying.
	EnsureDead([]names.Tag) ([]params.ErrorResult, error)

	// Remove removes the entities with the specified tags from state.
	Remove([]names.Tag) ([]params.ErrorResult, error)
}

// NewStorageProvisioner returns a Worker which manages
// provisioning (deprovisioning), and attachment (detachment)
// of first-class volumes and filesystems.
//
// Machine-scoped storage workers will be provided with
// a storage directory, while environment-scoped workers
// will not. If the directory path is non-empty, then it
// will be passed to the storage source via its config.
func NewStorageProvisioner(storageDir string, v VolumeAccessor, l LifecycleManager) worker.Worker {
	w := &storageprovisioner{
		storageDir: storageDir,
		volumes:    v,
		life:       l,
	}
	go func() {
		defer w.tomb.Done()
		w.tomb.Kill(w.loop())
	}()
	return w
}

type storageprovisioner struct {
	tomb       tomb.Tomb
	storageDir string
	volumes    VolumeAccessor
	life       LifecycleManager
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
	// TODO(axw) wait for and watch environ config.
	var environConfig *config.Config
	/*
		var environConfigChanges <-chan struct{}
		environWatcher, err := p.st.WatchForEnvironConfigChanges()
		if err != nil {
			return err
		}
		environConfigChanges = environWatcher.Changes()
		defer watcher.Stop(environWatcher, &p.tomb)
		p.environ, err = worker.WaitForEnviron(environWatcher, p.st, p.tomb.Dying())
		if err != nil {
			return err
		}
	*/

	volumesWatcher, err := w.volumes.WatchVolumes()
	if err != nil {
		return errors.Annotate(err, "watching volumes")
	}
	defer watcher.Stop(volumesWatcher, &w.tomb)
	volumesChanges := volumesWatcher.Changes()

	ctx := context{
		environConfig: environConfig,
		storageDir:    w.storageDir,
		volumes:       w.volumes,
		life:          w.life,
	}

	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case changes, ok := <-volumesChanges:
			if !ok {
				return watcher.EnsureErr(volumesWatcher)
			}
			if err := volumesChanged(&ctx, changes); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

type context struct {
	environConfig *config.Config
	storageDir    string
	volumes       VolumeAccessor
	life          LifecycleManager
}
