// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure_test

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v2"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"github.com/microsoftgraph/msgraph-sdk-go/models/odataerrors"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/provider/azure"
	"github.com/juju/juju/internal/provider/azure/internal/azuretesting"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/testing"
)

type storageSuite struct {
	testing.BaseSuite

	provider storage.Provider
	requests []*http.Request
	sender   azuretesting.Senders

	credentialInvalidator environs.CredentialInvalidator
	invalidatedCredential bool
}

var _ = tc.Suite(&storageSuite{})

func (s *storageSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.requests = nil
	envProvider := newProvider(c, azure.ProviderConfig{
		Sender:           &s.sender,
		RequestInspector: &azuretesting.RequestRecorderPolicy{Requests: &s.requests},
		CreateTokenCredential: func(appId, appPassword, tenantID string, opts azcore.ClientOptions) (azcore.TokenCredential, error) {
			return &azuretesting.FakeCredential{}, nil
		},
	})
	s.sender = nil

	var err error
	env := openEnviron(c, envProvider, s.credentialInvalidator, &s.sender)
	s.provider, err = env.StorageProvider("azure")
	c.Assert(err, jc.ErrorIsNil)

	s.invalidatedCredential = false
	s.credentialInvalidator = azure.CredentialInvalidator(func(context.Context, environs.CredentialInvalidReason) error {
		s.invalidatedCredential = true
		return nil
	})
}

func (s *storageSuite) volumeSource(c *tc.C, attrs ...testing.Attrs) storage.VolumeSource {
	storageConfig, err := storage.NewConfig("azure", "azure", nil)
	c.Assert(err, jc.ErrorIsNil)

	s.sender = azuretesting.Senders{}
	volumeSource, err := s.provider.VolumeSource(storageConfig)
	c.Assert(err, jc.ErrorIsNil)
	return volumeSource
}

func (s *storageSuite) TestVolumeSource(c *tc.C) {
	vs := s.volumeSource(c)
	c.Assert(vs, tc.NotNil)
}

func (s *storageSuite) TestFilesystemSource(c *tc.C) {
	storageConfig, err := storage.NewConfig("azure", "azure", nil)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.provider.FilesystemSource(storageConfig)
	c.Assert(err, tc.ErrorMatches, "filesystems not supported")
	c.Assert(err, jc.ErrorIs, errors.NotSupported)
}

func (s *storageSuite) TestSupports(c *tc.C) {
	c.Assert(s.provider.Supports(storage.StorageKindBlock), jc.IsTrue)
	c.Assert(s.provider.Supports(storage.StorageKindFilesystem), jc.IsFalse)
}

func (s *storageSuite) TestDynamic(c *tc.C) {
	c.Assert(s.provider.Dynamic(), jc.IsTrue)
}

func (s *storageSuite) TestScope(c *tc.C) {
	c.Assert(s.provider.Scope(), tc.Equals, storage.ScopeEnviron)
}

func (s *storageSuite) TestCreateVolumes(c *tc.C) {
	makeVolumeParams := func(volume, machine string, size uint64) storage.VolumeParams {
		return storage.VolumeParams{
			Tag:          names.NewVolumeTag(volume),
			Size:         size,
			Provider:     "azure",
			ResourceTags: map[string]string{"foo": "bar"},
			Attachment: &storage.VolumeAttachmentParams{
				AttachmentParams: storage.AttachmentParams{
					Provider:   "azure",
					Machine:    names.NewMachineTag(machine),
					InstanceId: instance.Id("machine-" + machine),
				},
				Volume: names.NewVolumeTag(volume),
			},
		}
	}
	params := []storage.VolumeParams{
		makeVolumeParams("0", "0", 1),
		makeVolumeParams("1", "1", 1025),
		makeVolumeParams("2", "0", 1024),
	}

	makeSender := func(name string, sizeGB int32) *azuretesting.MockSender {
		sender := azuretesting.NewSenderWithValue(&armcompute.Disk{
			Name: to.Ptr(name),
			Properties: &armcompute.DiskProperties{
				DiskSizeGB: to.Ptr(sizeGB),
			},
		})
		sender.PathPattern = `.*/Microsoft\.Compute/disks/` + name
		return sender
	}

	volumeSource := s.volumeSource(c)
	s.requests = nil
	s.sender = azuretesting.Senders{
		makeSender("volume-0", 32),
		makeSender("volume-1", 2),
		makeSender("volume-2", 1),
	}

	results, err := volumeSource.CreateVolumes(context.Background(), params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, tc.HasLen, len(params))
	c.Check(results[0].Error, jc.ErrorIsNil)
	c.Check(results[1].Error, jc.ErrorIsNil)
	c.Check(results[2].Error, jc.ErrorIsNil)

	// Attachments are deferred.
	c.Check(results[0].VolumeAttachment, tc.IsNil)
	c.Check(results[1].VolumeAttachment, tc.IsNil)
	c.Check(results[2].VolumeAttachment, tc.IsNil)

	makeVolume := func(id string, size uint64) *storage.Volume {
		return &storage.Volume{
			Tag: names.NewVolumeTag(id),
			VolumeInfo: storage.VolumeInfo{
				Size:       size,
				VolumeId:   "volume-" + id,
				Persistent: true,
			},
		}
	}
	c.Check(results[0].Volume, jc.DeepEquals, makeVolume("0", 32*1024))
	c.Check(results[1].Volume, jc.DeepEquals, makeVolume("1", 2*1024))
	c.Check(results[2].Volume, jc.DeepEquals, makeVolume("2", 1*1024))

	// Validate HTTP request bodies.
	c.Assert(s.requests, tc.HasLen, 3)
	c.Assert(s.requests[0].Method, tc.Equals, "PUT") // create volume-0
	c.Assert(s.requests[1].Method, tc.Equals, "PUT") // create volume-1
	c.Assert(s.requests[2].Method, tc.Equals, "PUT") // create volume-2

	makeDisk := func(name string, size int32) *armcompute.Disk {
		tags := map[string]*string{
			"foo": to.Ptr("bar"),
		}
		return &armcompute.Disk{
			Name:     to.Ptr(name),
			Location: to.Ptr("westus"),
			Tags:     tags,
			SKU: &armcompute.DiskSKU{
				Name: to.Ptr(armcompute.DiskStorageAccountTypesStandardLRS),
			},
			Properties: &armcompute.DiskProperties{
				DiskSizeGB: to.Ptr(size),
				CreationData: &armcompute.CreationData{
					CreateOption: to.Ptr(armcompute.DiskCreateOptionEmpty),
				},
			},
		}
	}
	// Only check the PUT requests.
	assertRequestBody(c, s.requests[0], makeDisk("volume-0", 1))
	assertRequestBody(c, s.requests[1], makeDisk("volume-1", 2))
	assertRequestBody(c, s.requests[2], makeDisk("volume-2", 1))
}

func (s *storageSuite) createSenderWithUnauthorisedStatusCode() {
	unauthSender := &azuretesting.MockSender{}
	unauthSender.AppendAndRepeatResponse(azuretesting.NewResponseWithStatus("401 Unauthorized", http.StatusUnauthorized), 3)
	s.sender = azuretesting.Senders{unauthSender, unauthSender, unauthSender}
}

func (s *storageSuite) TestCreateVolumesWithInvalidCredential(c *tc.C) {
	makeVolumeParams := func(volume, machine string, size uint64) storage.VolumeParams {
		return storage.VolumeParams{
			Tag:          names.NewVolumeTag(volume),
			Size:         size,
			Provider:     "azure",
			ResourceTags: map[string]string{"foo": "bar"},
			Attachment: &storage.VolumeAttachmentParams{
				AttachmentParams: storage.AttachmentParams{
					Provider:   "azure",
					Machine:    names.NewMachineTag(machine),
					InstanceId: instance.Id("machine-" + machine),
				},
				Volume: names.NewVolumeTag(volume),
			},
		}
	}
	params := []storage.VolumeParams{
		makeVolumeParams("0", "0", 1),
		makeVolumeParams("1", "1", 1025),
		makeVolumeParams("2", "0", 1024),
	}

	volumeSource := s.volumeSource(c)
	s.requests = nil
	s.createSenderWithUnauthorisedStatusCode()

	c.Assert(s.invalidatedCredential, jc.IsFalse)
	results, err := volumeSource.CreateVolumes(context.Background(), params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, tc.HasLen, len(params))
	c.Check(results[0].Error, tc.NotNil)
	c.Check(results[1].Error, tc.NotNil)
	c.Check(results[2].Error, tc.NotNil)

	// Attachments are deferred.
	c.Check(results[0].VolumeAttachment, tc.IsNil)
	c.Check(results[1].VolumeAttachment, tc.IsNil)
	c.Check(results[2].VolumeAttachment, tc.IsNil)
	c.Assert(s.invalidatedCredential, jc.IsTrue)

	// Validate HTTP request bodies.
	// The authorised workflow attempts to refresh to token so
	// there's additional requests to account for as well.
	c.Assert(s.requests, tc.HasLen, 3)
	c.Assert(s.requests[0].Method, tc.Equals, "PUT") // create volume-0
	c.Assert(s.requests[1].Method, tc.Equals, "PUT") // create volume-1
	c.Assert(s.requests[2].Method, tc.Equals, "PUT") // create volume-2

	makeDisk := func(name string, size int32) *armcompute.Disk {
		tags := map[string]*string{
			"foo": to.Ptr("bar"),
		}
		return &armcompute.Disk{
			Name:     to.Ptr(name),
			Location: to.Ptr("westus"),
			Tags:     tags,
			Properties: &armcompute.DiskProperties{
				DiskSizeGB: to.Ptr(size),
				CreationData: &armcompute.CreationData{
					CreateOption: to.Ptr(armcompute.DiskCreateOptionEmpty),
				},
			},
			SKU: &armcompute.DiskSKU{
				Name: to.Ptr(armcompute.DiskStorageAccountTypesStandardLRS),
			},
		}
	}
	assertRequestBody(c, s.requests[0], makeDisk("volume-0", 1))
	assertRequestBody(c, s.requests[1], makeDisk("volume-1", 2))
	assertRequestBody(c, s.requests[2], makeDisk("volume-2", 1))
}

func (s *storageSuite) TestListVolumes(c *tc.C) {
	volumeSource := s.volumeSource(c)
	disks := []*armcompute.Disk{{
		Name: to.Ptr("volume-0"),
	}, {
		Name: to.Ptr("machine-0"),
	}, {
		Name: to.Ptr("volume-1"),
	}}
	volumeSender := azuretesting.NewSenderWithValue(armcompute.DiskList{
		Value: disks,
	})
	volumeSender.PathPattern = `.*/Microsoft\.Compute/disks`
	s.sender = azuretesting.Senders{volumeSender}

	volumeIds, err := volumeSource.ListVolumes(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumeIds, jc.SameContents, []string{"volume-0", "volume-1"})
}

func (s *storageSuite) TestListVolumesWithInvalidCredential(c *tc.C) {
	volumeSource := s.volumeSource(c)
	s.createSenderWithUnauthorisedStatusCode()

	c.Assert(s.invalidatedCredential, jc.IsFalse)
	_, err := volumeSource.ListVolumes(context.Background())
	c.Assert(err, tc.NotNil)
	c.Assert(s.invalidatedCredential, jc.IsTrue)
}

func (s *storageSuite) TestListVolumesErrors(c *tc.C) {
	volumeSource := s.volumeSource(c)
	sender := &azuretesting.MockSender{}
	sender.SetAndRepeatError(errors.New("no disks for you"), -1)
	s.sender = azuretesting.Senders{
		sender,
		sender, // for the retry attempt
	}
	_, err := volumeSource.ListVolumes(context.Background())
	c.Assert(err, tc.ErrorMatches, ".*listing disks: no disks for you")
}

func (s *storageSuite) TestDescribeVolumes(c *tc.C) {
	volumeSource := s.volumeSource(c)
	volumeSender := azuretesting.NewSenderWithValue(&armcompute.Disk{
		Properties: &armcompute.DiskProperties{
			DiskSizeGB: to.Ptr(int32(1024)),
		},
	})
	volumeSender.PathPattern = `.*/Microsoft\.Compute/disks/volume-0`
	s.sender = azuretesting.Senders{volumeSender}

	results, err := volumeSource.DescribeVolumes(context.Background(), []string{"volume-0"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, []storage.DescribeVolumesResult{{
		VolumeInfo: &storage.VolumeInfo{
			VolumeId:   "volume-0",
			Size:       1024 * 1024,
			Persistent: true,
		},
	}})
}

func (s *storageSuite) TestDescribeVolumesWithInvalidCredential(c *tc.C) {
	volumeSource := s.volumeSource(c)
	s.createSenderWithUnauthorisedStatusCode()

	c.Assert(s.invalidatedCredential, jc.IsFalse)
	_, err := volumeSource.DescribeVolumes(context.Background(), []string{"volume-0"})
	c.Assert(err, jc.ErrorIsNil)
	results, err := volumeSource.DescribeVolumes(context.Background(), []string{"volume-0"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results[0].Error, tc.NotNil)
	c.Assert(s.invalidatedCredential, jc.IsTrue)
}

func (s *storageSuite) TestDescribeVolumesNotFound(c *tc.C) {
	volumeSource := s.volumeSource(c)
	volumeSender := &azuretesting.MockSender{}
	response := azuretesting.NewResponseWithBodyAndStatus(
		azuretesting.NewBody("{}"),
		http.StatusNotFound,
		"disk not found",
	)
	volumeSender.AppendResponse(response)
	s.sender = azuretesting.Senders{volumeSender}
	results, err := volumeSource.DescribeVolumes(context.Background(), []string{"volume-42"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Assert(results[0].Error, jc.ErrorIs, errors.NotFound)
	c.Assert(results[0].Error, tc.ErrorMatches, `.*disk volume-42 not found`)
}

func (s *storageSuite) TestDestroyVolumes(c *tc.C) {
	volumeSource := s.volumeSource(c)

	volume0Sender := azuretesting.NewSenderWithValue(&odataerrors.ODataError{})
	volume0Sender.PathPattern = `.*/Microsoft\.Compute/disks/volume-0`
	s.sender = azuretesting.Senders{volume0Sender}

	results, err := volumeSource.DestroyVolumes(context.Background(), []string{"volume-0"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Assert(results[0], jc.ErrorIsNil)
}

func (s *storageSuite) TestDestroyVolumesWithInvalidCredential(c *tc.C) {
	volumeSource := s.volumeSource(c)

	s.createSenderWithUnauthorisedStatusCode()
	c.Assert(s.invalidatedCredential, jc.IsFalse)
	results, err := volumeSource.DestroyVolumes(context.Background(), []string{"volume-0"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Assert(results[0], tc.NotNil)
	c.Assert(s.invalidatedCredential, jc.IsTrue)
}

func (s *storageSuite) TestDestroyVolumesNotFound(c *tc.C) {
	volumeSource := s.volumeSource(c)

	volume42Sender := &azuretesting.MockSender{}
	volume42Sender.AppendResponse(azuretesting.NewResponseWithStatus(
		"disk not found", http.StatusNotFound,
	))
	s.sender = azuretesting.Senders{volume42Sender}

	results, err := volumeSource.DestroyVolumes(context.Background(), []string{"volume-42"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Assert(results[0], jc.ErrorIsNil)
}

func (s *storageSuite) TestAttachVolumes(c *tc.C) {
	// machine-1 has a single data disk with LUN 0.
	machine1DataDisks := []*armcompute.DataDisk{{
		Lun:  to.Ptr(int32(0)),
		Name: to.Ptr("volume-1"),
	}}
	// machine-2 has 32 data disks; no LUNs free.
	machine2DataDisks := make([]*armcompute.DataDisk, 32)
	for i := range machine2DataDisks {
		machine2DataDisks[i] = &armcompute.DataDisk{
			Lun:  to.Ptr(int32(i)),
			Name: to.Ptr(fmt.Sprintf("volume-%d", i)),
		}
	}

	// volume-0 and volume-2 are attached to machine-0
	// volume-1 is attached to machine-1
	// volume-3 is attached to machine-42, but machine-42 is missing
	// volume-42 is attached to machine-2, but machine-2 has no free LUNs
	makeParams := func(volume, machine string, size uint64) storage.VolumeAttachmentParams {
		return storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Provider:   "azure",
				Machine:    names.NewMachineTag(machine),
				InstanceId: instance.Id("machine-" + machine),
			},
			Volume:   names.NewVolumeTag(volume),
			VolumeId: "volume-" + volume,
		}
	}
	params := []storage.VolumeAttachmentParams{
		makeParams("0", "0", 1),
		makeParams("1", "1", 1025),
		makeParams("2", "0", 1024),
		makeParams("3", "42", 40),
		makeParams("42", "2", 50),
	}

	virtualMachines := []*armcompute.VirtualMachine{{
		Name: to.Ptr("machine-0"),
		Properties: &armcompute.VirtualMachineProperties{
			StorageProfile: &armcompute.StorageProfile{},
		},
	}, {
		Name: to.Ptr("machine-1"),
		Properties: &armcompute.VirtualMachineProperties{
			StorageProfile: &armcompute.StorageProfile{DataDisks: machine1DataDisks},
		},
	}, {
		Name: to.Ptr("machine-2"),
		Properties: &armcompute.VirtualMachineProperties{
			StorageProfile: &armcompute.StorageProfile{DataDisks: machine2DataDisks},
		},
	}}

	// There should be a one API calls to list VMs, and one update per modified instance.
	virtualMachinesSender := azuretesting.NewSenderWithValue(armcompute.VirtualMachineListResult{
		Value: virtualMachines,
	})
	virtualMachinesSender.PathPattern = `.*/Microsoft\.Compute/virtualMachines`
	updateVirtualMachine0Sender := azuretesting.NewSenderWithValue(&armcompute.VirtualMachine{})
	updateVirtualMachine0Sender.PathPattern = `.*/Microsoft\.Compute/virtualMachines/machine-0`

	volumeSource := s.volumeSource(c)
	s.requests = nil
	s.sender = azuretesting.Senders{
		virtualMachinesSender,
		updateVirtualMachine0Sender,
		updateVirtualMachine0Sender,
	}

	results, err := volumeSource.AttachVolumes(context.Background(), params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, tc.HasLen, len(params))

	c.Check(results[0].Error, jc.ErrorIsNil)
	c.Check(results[1].Error, jc.ErrorIsNil)
	c.Check(results[2].Error, jc.ErrorIsNil)
	c.Check(results[3].Error, tc.ErrorMatches, "instance machine-42 not found")
	c.Check(results[4].Error, tc.ErrorMatches, "choosing LUN: all LUNs are in use")

	// Validate HTTP request bodies.
	c.Assert(s.requests, tc.HasLen, 2)
	c.Assert(s.requests[0].Method, tc.Equals, "GET") // list virtual machines
	c.Assert(s.requests[1].Method, tc.Equals, "PUT") // update machine-0

	makeManagedDisk := func(volumeName string) *armcompute.ManagedDiskParameters {
		return &armcompute.ManagedDiskParameters{
			ID: to.Ptr(fmt.Sprintf("/subscriptions/%s/resourceGroups/juju-testmodel-deadbeef/providers/Microsoft.Compute/disks/%s", fakeManagedSubscriptionId, volumeName)),
		}
	}

	machine0DataDisks := []*armcompute.DataDisk{{
		Lun:          to.Ptr(int32(0)),
		Name:         to.Ptr("volume-0"),
		ManagedDisk:  makeManagedDisk("volume-0"),
		Caching:      to.Ptr(armcompute.CachingTypesReadWrite),
		CreateOption: to.Ptr(armcompute.DiskCreateOptionTypesAttach),
	}, {
		Lun:          to.Ptr(int32(1)),
		Name:         to.Ptr("volume-2"),
		ManagedDisk:  makeManagedDisk("volume-2"),
		Caching:      to.Ptr(armcompute.CachingTypesReadWrite),
		CreateOption: to.Ptr(armcompute.DiskCreateOptionTypesAttach),
	}}

	assertRequestBody(c, s.requests[1], &armcompute.VirtualMachine{
		Name: to.Ptr("machine-0"),
		Properties: &armcompute.VirtualMachineProperties{
			StorageProfile: &armcompute.StorageProfile{
				DataDisks: machine0DataDisks,
			},
		},
	})
}

func (s *storageSuite) TestDetachVolumes(c *tc.C) {
	// machine-0 has a three data disks: volume-0, volume-1 and volume-2
	machine0DataDisks := []*armcompute.DataDisk{{
		Lun:  to.Ptr(int32(0)),
		Name: to.Ptr("volume-0"),
	}, {
		Lun:  to.Ptr(int32(1)),
		Name: to.Ptr("volume-1"),
	}, {
		Lun:  to.Ptr(int32(2)),
		Name: to.Ptr("volume-2"),
	}}

	makeParams := func(volume, machine string) storage.VolumeAttachmentParams {
		return storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Provider:   "azure",
				Machine:    names.NewMachineTag(machine),
				InstanceId: instance.Id("machine-" + machine),
			},
			Volume:   names.NewVolumeTag(volume),
			VolumeId: "volume-" + volume,
		}
	}
	params := []storage.VolumeAttachmentParams{
		makeParams("1", "0"),
		makeParams("1", "0"),
		makeParams("42", "1"),
		makeParams("2", "42"),
	}

	virtualMachines := []*armcompute.VirtualMachine{{
		Name: to.Ptr("machine-0"),
		Properties: &armcompute.VirtualMachineProperties{
			StorageProfile: &armcompute.StorageProfile{DataDisks: machine0DataDisks},
		},
	}, {
		Name: to.Ptr("machine-1"),
		Properties: &armcompute.VirtualMachineProperties{
			StorageProfile: &armcompute.StorageProfile{},
		},
	}}

	// There should be a one API calls to list VMs, and one update per modified instance.
	virtualMachinesSender := azuretesting.NewSenderWithValue(armcompute.VirtualMachineListResult{
		Value: virtualMachines,
	})
	virtualMachinesSender.PathPattern = `.*/Microsoft\.Compute/virtualMachines`
	updateVirtualMachine0Sender := azuretesting.NewSenderWithValue(&armcompute.VirtualMachine{})
	updateVirtualMachine0Sender.PathPattern = `.*/Microsoft\.Compute/virtualMachines/machine-0`

	volumeSource := s.volumeSource(c)
	s.requests = nil
	s.sender = azuretesting.Senders{
		virtualMachinesSender,
		updateVirtualMachine0Sender,
		updateVirtualMachine0Sender,
	}

	results, err := volumeSource.DetachVolumes(context.Background(), params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, tc.HasLen, len(params))

	c.Check(results[0], jc.ErrorIsNil)
	c.Check(results[1], jc.ErrorIsNil)
	c.Check(results[2], jc.ErrorIsNil)
	c.Check(results[3], tc.ErrorMatches, "instance machine-42 not found")

	// Validate HTTP request bodies.
	c.Assert(s.requests, tc.HasLen, 2)
	c.Assert(s.requests[0].Method, tc.Equals, "GET") // list virtual machines
	c.Assert(s.requests[1].Method, tc.Equals, "PUT") // update machine-0

	assertRequestBody(c, s.requests[1], &armcompute.VirtualMachine{
		Name: to.Ptr("machine-0"),
		Properties: &armcompute.VirtualMachineProperties{
			StorageProfile: &armcompute.StorageProfile{
				DataDisks: []*armcompute.DataDisk{
					machine0DataDisks[0],
					machine0DataDisks[2],
				},
			},
		},
	})
}

func (s *storageSuite) TestDetachVolumesFinal(c *tc.C) {
	// machine-0 has a one data disk: volume-0.
	machine0DataDisks := []*armcompute.DataDisk{{
		Lun:  to.Ptr(int32(0)),
		Name: to.Ptr("volume-0"),
	}}

	params := []storage.VolumeAttachmentParams{{
		AttachmentParams: storage.AttachmentParams{
			Provider:   "azure",
			Machine:    names.NewMachineTag("0"),
			InstanceId: instance.Id("machine-0"),
		},
		Volume:   names.NewVolumeTag("0"),
		VolumeId: "volume-0",
	}}

	virtualMachines := []*armcompute.VirtualMachine{{
		Name: to.Ptr("machine-0"),
		Properties: &armcompute.VirtualMachineProperties{
			StorageProfile: &armcompute.StorageProfile{DataDisks: machine0DataDisks},
		},
	}}

	// There should be a one API call to list VMs, and one update to the VM.
	virtualMachinesSender := azuretesting.NewSenderWithValue(armcompute.VirtualMachineListResult{
		Value: virtualMachines,
	})
	virtualMachinesSender.PathPattern = `.*/Microsoft\.Compute/virtualMachines`
	updateVirtualMachine0Sender := azuretesting.NewSenderWithValue(&armcompute.VirtualMachine{})
	updateVirtualMachine0Sender.PathPattern = `.*/Microsoft\.Compute/virtualMachines/machine-0`

	volumeSource := s.volumeSource(c)
	s.requests = nil
	s.sender = azuretesting.Senders{
		virtualMachinesSender,
		updateVirtualMachine0Sender,
	}

	results, err := volumeSource.DetachVolumes(context.Background(), params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, tc.HasLen, len(params))
	c.Assert(results[0], jc.ErrorIsNil)

	// Validate HTTP request bodies.
	c.Assert(s.requests, tc.HasLen, 2)
	c.Assert(s.requests[0].Method, tc.Equals, "GET") // list virtual machines
	c.Assert(s.requests[1].Method, tc.Equals, "PUT") // update machine-0

	assertRequestBody(c, s.requests[1], &armcompute.VirtualMachine{
		Name: to.Ptr("machine-0"),
		Properties: &armcompute.VirtualMachineProperties{
			StorageProfile: &armcompute.StorageProfile{
				DataDisks: []*armcompute.DataDisk{},
			},
		},
	})
}
