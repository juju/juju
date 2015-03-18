// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	gc "gopkg.in/check.v1"

	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider/registry"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/storageprovisioner"
)

type storageProvisionerSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&storageProvisionerSuite{})

type mockStringsWatcher struct {
	changes <-chan []string
}

func (*mockStringsWatcher) Stop() error {
	return nil
}

func (*mockStringsWatcher) Err() error {
	return nil
}

func (w *mockStringsWatcher) Changes() <-chan []string {
	return w.changes
}

type mockVolumeAccessor struct {
	mockStringsWatcher apiwatcher.StringsWatcher
	provisioned        map[string]params.Volume
	done               chan struct{}
	// If SetVolumeInfo is called with expectedVolumes, then the
	// volume creation is as expected and the done channel is closed.
	expectedVolumes []params.Volume
}

func (w *mockVolumeAccessor) WatchVolumes() (apiwatcher.StringsWatcher, error) {
	return w.mockStringsWatcher, nil
}

func (v *mockVolumeAccessor) Volumes(volumes []names.VolumeTag) ([]params.VolumeResult, error) {
	var result []params.VolumeResult
	for _, tag := range volumes {
		if vol, ok := v.provisioned[tag.String()]; ok {
			result = append(result, params.VolumeResult{Result: vol})
		} else {
			result = append(result, params.VolumeResult{
				Error: common.ServerError(errors.NotProvisionedf("volume %q", tag.Id())),
			})
		}
	}
	return result, nil
}

func (v *mockVolumeAccessor) VolumeParams(volumes []names.VolumeTag) ([]params.VolumeParamsResult, error) {
	var result []params.VolumeParamsResult
	for _, tag := range volumes {
		if _, ok := v.provisioned[tag.String()]; ok {
			result = append(result, params.VolumeParamsResult{
				Error: &params.Error{Message: "already provisioned"},
			})
		} else {
			result = append(result, params.VolumeParamsResult{Result: params.VolumeParams{
				VolumeTag:  tag.String(),
				Size:       1024,
				Provider:   "dummy",
				Attributes: map[string]interface{}{"persistent": tag.String() == "volume-1"},
			}})
		}
	}
	return result, nil
}

func (v *mockVolumeAccessor) SetVolumeInfo(volumes []params.Volume) ([]params.ErrorResult, error) {
	for _, vol := range volumes {
		v.provisioned[vol.VolumeTag] = vol
	}
	// See if we have the expected volumes, using json serialisation to do the comparison.
	jsonVolInfo, err := json.Marshal(volumes)
	if err != nil {
		return []params.ErrorResult{{Error: common.ServerError(err)}}, nil
	}
	jsonExpectedInfo, err := json.Marshal(v.expectedVolumes)
	if err != nil {
		return []params.ErrorResult{{Error: common.ServerError(err)}}, nil
	}
	// If we have what we expect, close the done channel.
	if string(jsonVolInfo) == string(jsonExpectedInfo) {
		close(v.done)
	}
	return nil, nil
}

func newMockVolumeAccessor(changes <-chan []string, done chan struct{}, expectedVolumes []params.Volume) storageprovisioner.VolumeAccessor {
	return &mockVolumeAccessor{
		&mockStringsWatcher{changes},
		make(map[string]params.Volume),
		done,
		expectedVolumes,
	}
}

type mockLifecycleManager struct {
}

func (m *mockLifecycleManager) Life(volumes []names.Tag) ([]params.LifeResult, error) {
	var result []params.LifeResult
	for _, tag := range volumes {
		id, _ := strconv.Atoi(tag.Id())
		if id <= 100 {
			result = append(result, params.LifeResult{Life: params.Alive})
		} else {
			result = append(result, params.LifeResult{Life: params.Dying})
		}
	}
	return result, nil
}

func (m *mockLifecycleManager) EnsureDead([]names.Tag) ([]params.ErrorResult, error) {
	return nil, nil
}

func (m *mockLifecycleManager) Remove([]names.Tag) ([]params.ErrorResult, error) {
	return nil, nil
}

func (s *storageProvisionerSuite) TestStartStop(c *gc.C) {
	changes := make(chan []string)
	worker := storageprovisioner.NewStorageProvisioner(
		"dir", newMockVolumeAccessor(changes, nil, nil), &mockLifecycleManager{},
	)
	worker.Kill()
	c.Assert(worker.Wait(), gc.IsNil)
}

// Set up a dummy storage provider so we can stub out volume creation.
type dummyProvider struct {
	storage.Provider
}

type dummyVolumeSource struct {
	storage.VolumeSource
}

func (*dummyProvider) VolumeSource(environConfig *config.Config, providerConfig *storage.Config) (storage.VolumeSource, error) {
	return &dummyVolumeSource{}, nil
}

// CreateVolumes makes some volumes that we can check later to ensure things went as expected.
func (*dummyVolumeSource) CreateVolumes(params []storage.VolumeParams) ([]storage.Volume, []storage.VolumeAttachment, error) {
	var volumes []storage.Volume
	var volumeAttachments []storage.VolumeAttachment
	for _, p := range params {
		persistent, _ := p.Attributes["persistent"].(bool)
		volumes = append(volumes, storage.Volume{
			Tag:        p.Tag,
			Size:       p.Size,
			Serial:     "serial-" + p.Tag.Id(),
			VolumeId:   "id-" + p.Tag.Id(),
			Persistent: persistent,
		})
		volumeAttachments = append(volumeAttachments, storage.VolumeAttachment{
			Volume:     p.Tag,
			Machine:    names.NewMachineTag("0"),
			DeviceName: "/dev/sda" + p.Tag.Id(),
		})
	}
	return volumes, volumeAttachments, nil
}

func (s *storageProvisionerSuite) TestVolumeAdded(c *gc.C) {
	registry.RegisterProvider(storage.ProviderType("dummy"), &dummyProvider{})
	updated := make(chan struct{})
	changes := make(chan []string)
	expectedVolumes := []params.Volume{
		{VolumeTag: "volume-1", VolumeId: "id-1", Serial: "serial-1", Size: 1024, Persistent: true},
		{VolumeTag: "volume-2", VolumeId: "id-2", Serial: "serial-2", Size: 1024},
	}
	worker := storageprovisioner.NewStorageProvisioner(
		"storage-dir", newMockVolumeAccessor(changes, updated, expectedVolumes), &mockLifecycleManager{},
	)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	// The worker should create volumes according to ids "1" and "2".
	changes <- []string{"1", "2"}
	select {
	case <-updated:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for volume change to be processd")
	}
}

// TODO(wallyworld) - test destroying volumes when done
