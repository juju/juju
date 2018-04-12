// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure_test

import (
	"fmt"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/arm/compute"
	"github.com/Azure/azure-sdk-for-go/arm/disk"
	armstorage "github.com/Azure/azure-sdk-for-go/arm/storage"
	azurestorage "github.com/Azure/azure-sdk-for-go/storage"
	autorestazure "github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/mocks"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/azure"
	internalazurestorage "github.com/juju/juju/provider/azure/internal/azurestorage"
	"github.com/juju/juju/provider/azure/internal/azuretesting"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
)

type storageSuite struct {
	testing.BaseSuite

	datavhdsContainer azuretesting.MockStorageContainer
	storageClient     azuretesting.MockStorageClient
	provider          storage.Provider
	requests          []*http.Request
	sender            azuretesting.Senders
}

var _ = gc.Suite(&storageSuite{})

func (s *storageSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.datavhdsContainer = azuretesting.MockStorageContainer{}
	s.storageClient = azuretesting.MockStorageClient{
		Containers: map[string]internalazurestorage.Container{
			"datavhds": &s.datavhdsContainer,
		},
	}
	s.requests = nil
	envProvider := newProvider(c, azure.ProviderConfig{
		Sender:                     &s.sender,
		NewStorageClient:           s.storageClient.NewClient,
		RequestInspector:           azuretesting.RequestRecorder(&s.requests),
		RandomWindowsAdminPassword: func() string { return "sorandom" },
	})
	s.sender = nil

	var err error
	env := openEnviron(c, envProvider, &s.sender)
	s.provider, err = env.StorageProvider("azure")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *storageSuite) volumeSource(c *gc.C, legacy bool, attrs ...testing.Attrs) storage.VolumeSource {
	storageConfig, err := storage.NewConfig("azure", "azure", nil)
	c.Assert(err, jc.ErrorIsNil)

	s.sender = azuretesting.Senders{}
	if legacy {
		s.sender = append(s.sender, s.accountSender(), s.accountKeysSender())
	} else {
		s.sender = append(s.sender, s.accountNotFoundSender())
	}
	volumeSource, err := s.provider.VolumeSource(storageConfig)
	c.Assert(err, jc.ErrorIsNil)

	// Force an explicit refresh of the access token, so it isn't done
	// implicitly during the tests.
	s.sender = azuretesting.Senders{tokenRefreshSender()}
	err = azure.ForceVolumeSourceTokenRefresh(volumeSource)
	c.Assert(err, jc.ErrorIsNil)
	return volumeSource
}

func (s *storageSuite) accountNotFoundSender() *mocks.Sender {
	sender := mocks.NewSender()
	sender.AppendResponse(mocks.NewResponseWithStatus(
		"storage account not found", http.StatusNotFound,
	))
	return sender
}

func (s *storageSuite) accountSender() *azuretesting.MockSender {
	envTags := map[string]*string{
		"juju-model-uuid": to.StringPtr(testing.ModelTag.Id()),
	}
	account := armstorage.Account{
		Name: to.StringPtr(storageAccountName),
		Type: to.StringPtr("Standard_LRS"),
		Tags: &envTags,
		AccountProperties: &armstorage.AccountProperties{
			PrimaryEndpoints: &armstorage.Endpoints{
				Blob: to.StringPtr(fmt.Sprintf("https://%s.blob.storage.azurestack.local/", storageAccountName)),
			},
		},
	}
	accountSender := azuretesting.NewSenderWithValue(account)
	accountSender.PathPattern = ".*/storageAccounts/" + storageAccountName + ".*"
	return accountSender
}

func (s *storageSuite) accountKeysSender() *azuretesting.MockSender {
	keys := []armstorage.AccountKey{{
		KeyName:     to.StringPtr(fakeStorageAccountKey + "-name"),
		Value:       to.StringPtr(fakeStorageAccountKey),
		Permissions: armstorage.Full,
	}, {
		KeyName:     to.StringPtr("key2-name"),
		Value:       to.StringPtr("key2"),
		Permissions: armstorage.Full,
	}}
	result := armstorage.AccountListKeysResult{Keys: &keys}
	keysSender := azuretesting.NewSenderWithValue(&result)
	keysSender.PathPattern = ".*/storageAccounts/.*/listKeys"
	return keysSender
}

func (s *storageSuite) TestVolumeSource(c *gc.C) {
	vs := s.volumeSource(c, false)
	c.Assert(vs, gc.NotNil)
}

func (s *storageSuite) TestFilesystemSource(c *gc.C) {
	storageConfig, err := storage.NewConfig("azure", "azure", nil)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.provider.FilesystemSource(storageConfig)
	c.Assert(err, gc.ErrorMatches, "filesystems not supported")
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}

func (s *storageSuite) TestSupports(c *gc.C) {
	c.Assert(s.provider.Supports(storage.StorageKindBlock), jc.IsTrue)
	c.Assert(s.provider.Supports(storage.StorageKindFilesystem), jc.IsFalse)
}

func (s *storageSuite) TestDynamic(c *gc.C) {
	c.Assert(s.provider.Dynamic(), jc.IsTrue)
}

func (s *storageSuite) TestScope(c *gc.C) {
	c.Assert(s.provider.Scope(), gc.Equals, storage.ScopeEnviron)
}

func (s *storageSuite) TestCreateVolumes(c *gc.C) {
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
		sender := azuretesting.NewSenderWithValue(&disk.Model{
			Name: to.StringPtr(name),
			Properties: &disk.Properties{
				DiskSizeGB: to.Int32Ptr(sizeGB),
			},
		})
		sender.PathPattern = `.*/Microsoft\.Compute/disks/` + name
		return sender
	}

	volumeSource := s.volumeSource(c, false)
	s.requests = nil
	s.sender = azuretesting.Senders{
		makeSender("volume-0", 32),
		makeSender("volume-1", 2),
		makeSender("volume-2", 1),
	}

	results, err := volumeSource.CreateVolumes(params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, len(params))
	c.Check(results[0].Error, jc.ErrorIsNil)
	c.Check(results[1].Error, jc.ErrorIsNil)
	c.Check(results[2].Error, jc.ErrorIsNil)

	// Attachments are deferred.
	c.Check(results[0].VolumeAttachment, gc.IsNil)
	c.Check(results[1].VolumeAttachment, gc.IsNil)
	c.Check(results[2].VolumeAttachment, gc.IsNil)

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
	c.Assert(s.requests, gc.HasLen, 3)
	c.Assert(s.requests[0].Method, gc.Equals, "PUT") // create volume-0
	c.Assert(s.requests[1].Method, gc.Equals, "PUT") // create volume-1
	c.Assert(s.requests[2].Method, gc.Equals, "PUT") // create volume-2

	makeDisk := func(name string, size int32) *disk.Model {
		tags := map[string]*string{
			"foo": to.StringPtr("bar"),
		}
		return &disk.Model{
			Name:     to.StringPtr(name),
			Location: to.StringPtr("westus"),
			Tags:     &tags,
			Properties: &disk.Properties{
				AccountType: disk.StorageAccountTypes("Standard_LRS"),
				DiskSizeGB:  to.Int32Ptr(size),
				CreationData: &disk.CreationData{
					CreateOption: "Empty",
				},
			},
		}
	}
	assertRequestBody(c, s.requests[0], makeDisk("volume-0", 1))
	assertRequestBody(c, s.requests[1], makeDisk("volume-1", 2))
	assertRequestBody(c, s.requests[2], makeDisk("volume-2", 1))
}

func (s *storageSuite) TestCreateVolumesLegacy(c *gc.C) {
	// machine-1 has a single data disk with LUN 0.
	machine1DataDisks := []compute.DataDisk{{Lun: to.Int32Ptr(0)}}
	// machine-2 has 32 data disks; no LUNs free.
	machine2DataDisks := make([]compute.DataDisk, 32)
	for i := range machine2DataDisks {
		machine2DataDisks[i].Lun = to.Int32Ptr(int32(i))
	}

	// volume-0 and volume-2 are attached to machine-0
	// volume-1 is attached to machine-1
	// volume-3 is attached to machine-42, but machine-42 is missing
	// volume-42 is attached to machine-2, but machine-2 has no free LUNs
	makeVolumeParams := func(volume, machine string, size uint64) storage.VolumeParams {
		return storage.VolumeParams{
			Tag:      names.NewVolumeTag(volume),
			Size:     size,
			Provider: "azure",
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
		makeVolumeParams("3", "42", 40),
		makeVolumeParams("42", "2", 50),
	}

	virtualMachines := []compute.VirtualMachine{{
		Name: to.StringPtr("machine-0"),
		VirtualMachineProperties: &compute.VirtualMachineProperties{
			StorageProfile: &compute.StorageProfile{},
		},
	}, {
		Name: to.StringPtr("machine-1"),
		VirtualMachineProperties: &compute.VirtualMachineProperties{
			StorageProfile: &compute.StorageProfile{DataDisks: &machine1DataDisks},
		},
	}, {
		Name: to.StringPtr("machine-2"),
		VirtualMachineProperties: &compute.VirtualMachineProperties{
			StorageProfile: &compute.StorageProfile{DataDisks: &machine2DataDisks},
		},
	}}

	// There should be a one API calls to list VMs, and one update per modified instance.
	virtualMachinesSender := azuretesting.NewSenderWithValue(compute.VirtualMachineListResult{
		Value: &virtualMachines,
	})
	virtualMachinesSender.PathPattern = `.*/Microsoft\.Compute/virtualMachines`
	updateVirtualMachine0Sender := azuretesting.NewSenderWithValue(&compute.VirtualMachine{})
	updateVirtualMachine0Sender.PathPattern = `.*/Microsoft\.Compute/virtualMachines/machine-0`
	updateVirtualMachine1Sender := azuretesting.NewSenderWithValue(&compute.VirtualMachine{})
	updateVirtualMachine1Sender.PathPattern = `.*/Microsoft\.Compute/virtualMachines/machine-1`

	volumeSource := s.volumeSource(c, true)
	s.requests = nil
	s.sender = azuretesting.Senders{
		virtualMachinesSender,
		updateVirtualMachine0Sender,
		updateVirtualMachine1Sender,
	}

	results, err := volumeSource.CreateVolumes(params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, len(params))

	c.Check(results[0].Error, jc.ErrorIsNil)
	c.Check(results[1].Error, jc.ErrorIsNil)
	c.Check(results[2].Error, jc.ErrorIsNil)
	c.Check(results[3].Error, gc.ErrorMatches, "instance machine-42 not found")
	c.Check(results[4].Error, gc.ErrorMatches, "choosing LUN: all LUNs are in use")

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
	c.Check(results[0].Volume, jc.DeepEquals, makeVolume("0", 1024))
	c.Check(results[1].Volume, jc.DeepEquals, makeVolume("1", 2048))
	c.Check(results[2].Volume, jc.DeepEquals, makeVolume("2", 1024))

	// Attachments created at the same time.
	makeVolumeAttachment := func(volumeId, machineId string, lun int) *storage.VolumeAttachment {
		return &storage.VolumeAttachment{
			Volume:  names.NewVolumeTag(volumeId),
			Machine: names.NewMachineTag(machineId),
			VolumeAttachmentInfo: storage.VolumeAttachmentInfo{
				BusAddress: fmt.Sprintf("scsi@5:0.0.%d", lun),
			},
		}
	}
	c.Check(results[0].VolumeAttachment, jc.DeepEquals, makeVolumeAttachment("0", "0", 0))
	c.Check(results[1].VolumeAttachment, jc.DeepEquals, makeVolumeAttachment("1", "1", 1))
	c.Check(results[2].VolumeAttachment, jc.DeepEquals, makeVolumeAttachment("2", "0", 1))

	// Validate HTTP request bodies.
	c.Assert(s.requests, gc.HasLen, 3)
	c.Assert(s.requests[0].Method, gc.Equals, "GET") // list virtual machines
	c.Assert(s.requests[1].Method, gc.Equals, "PUT") // update machine-0
	c.Assert(s.requests[2].Method, gc.Equals, "PUT") // update machine-1

	machine0DataDisks := []compute.DataDisk{{
		Lun:        to.Int32Ptr(0),
		DiskSizeGB: to.Int32Ptr(1),
		Name:       to.StringPtr("volume-0"),
		Vhd: &compute.VirtualHardDisk{URI: to.StringPtr(fmt.Sprintf(
			"https://%s.blob.storage.azurestack.local/datavhds/volume-0.vhd",
			storageAccountName,
		))},
		Caching:      compute.ReadWrite,
		CreateOption: compute.Empty,
	}, {
		Lun:        to.Int32Ptr(1),
		DiskSizeGB: to.Int32Ptr(1),
		Name:       to.StringPtr("volume-2"),
		Vhd: &compute.VirtualHardDisk{URI: to.StringPtr(fmt.Sprintf(
			"https://%s.blob.storage.azurestack.local/datavhds/volume-2.vhd",
			storageAccountName,
		))},
		Caching:      compute.ReadWrite,
		CreateOption: compute.Empty,
	}}
	virtualMachines[0].StorageProfile.DataDisks = &machine0DataDisks
	assertRequestBody(c, s.requests[1], &virtualMachines[0])

	machine1DataDisks = append(machine1DataDisks, compute.DataDisk{
		Lun:        to.Int32Ptr(1),
		DiskSizeGB: to.Int32Ptr(2),
		Name:       to.StringPtr("volume-1"),
		Vhd: &compute.VirtualHardDisk{URI: to.StringPtr(fmt.Sprintf(
			"https://%s.blob.storage.azurestack.local/datavhds/volume-1.vhd",
			storageAccountName,
		))},
		Caching:      compute.ReadWrite,
		CreateOption: compute.Empty,
	})
	assertRequestBody(c, s.requests[2], &virtualMachines[1])
}

func (s *storageSuite) TestListVolumes(c *gc.C) {
	volumeSource := s.volumeSource(c, false)
	disks := []disk.Model{{
		Name: to.StringPtr("volume-0"),
	}, {
		Name: to.StringPtr("machine-0"),
	}, {
		Name: to.StringPtr("volume-1"),
	}}
	volumeSender := azuretesting.NewSenderWithValue(&disk.ListType{
		Value: &disks,
	})
	volumeSender.PathPattern = `.*/Microsoft\.Compute/disks`
	s.sender = azuretesting.Senders{volumeSender}

	volumeIds, err := volumeSource.ListVolumes()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumeIds, jc.SameContents, []string{"volume-0", "volume-1"})
}

func (s *storageSuite) TestListVolumesLegacy(c *gc.C) {
	blob0 := &azuretesting.MockStorageBlob{
		Name_: "volume-0.vhd",
		Properties_: azurestorage.BlobProperties{
			ContentLength: 1024 * 1024 * 1024 * 1024, // 1TiB
		},
	}
	blob1 := &azuretesting.MockStorageBlob{
		Name_: "volume-1.vhd",
		Properties_: azurestorage.BlobProperties{
			ContentLength: 1024 * 1024, // 1MiB
		},
	}
	junkBlob := &azuretesting.MockStorageBlob{
		Name_: "junk.vhd",
	}
	volumeBlob := &azuretesting.MockStorageBlob{
		Name_: "volume",
	}
	s.datavhdsContainer.Blobs_ = []internalazurestorage.Blob{blob1, blob0, junkBlob, volumeBlob}

	volumeSource := s.volumeSource(c, true)
	volumeIds, err := volumeSource.ListVolumes()
	c.Assert(err, jc.ErrorIsNil)
	s.storageClient.CheckCallNames(c, "NewClient", "GetContainerReference")
	s.storageClient.CheckCall(
		c, 0, "NewClient", storageAccountName, fakeStorageAccountKey,
		"storage.azurestack.local", azurestorage.DefaultAPIVersion, true,
	)
	s.storageClient.CheckCall(c, 1, "GetContainerReference", "datavhds")
	s.datavhdsContainer.CheckCallNames(c, "Blobs")
	c.Assert(volumeIds, jc.DeepEquals, []string{"volume-1", "volume-0"})
}

func (s *storageSuite) TestListVolumesErrors(c *gc.C) {
	volumeSource := s.volumeSource(c, false)
	sender := mocks.NewSender()
	sender.SetError(errors.New("no disks for you"))
	s.sender = azuretesting.Senders{sender}
	_, err := volumeSource.ListVolumes()
	c.Assert(err, gc.ErrorMatches, "listing disks: .*: no disks for you")
}

func (s *storageSuite) TestListVolumesErrorsLegacy(c *gc.C) {
	volumeSource := s.volumeSource(c, true)
	s.datavhdsContainer.SetErrors(errors.New("no blobs for you"))
	_, err := volumeSource.ListVolumes()
	c.Assert(err, gc.ErrorMatches, "listing volumes: listing blobs: no blobs for you")
}

func (s *storageSuite) TestDescribeVolumes(c *gc.C) {
	volumeSource := s.volumeSource(c, false)
	volumeSender := azuretesting.NewSenderWithValue(&disk.Model{
		Properties: &disk.Properties{
			DiskSizeGB: to.Int32Ptr(1024),
		},
	})
	volumeSender.PathPattern = `.*/Microsoft\.Compute/disks/volume-0`
	s.sender = azuretesting.Senders{volumeSender}

	results, err := volumeSource.DescribeVolumes([]string{"volume-0"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, []storage.DescribeVolumesResult{{
		VolumeInfo: &storage.VolumeInfo{
			VolumeId:   "volume-0",
			Size:       1024 * 1024,
			Persistent: true,
		},
	}})
}

func (s *storageSuite) TestDescribeVolumesNotFound(c *gc.C) {
	volumeSource := s.volumeSource(c, false)
	volumeSender := mocks.NewSender()
	response := mocks.NewResponseWithBodyAndStatus(
		mocks.NewBody("{}"),
		http.StatusNotFound,
		"disk not found",
	)
	volumeSender.AppendResponse(response)
	s.sender = azuretesting.Senders{volumeSender}
	results, err := volumeSource.DescribeVolumes([]string{"volume-42"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Error, jc.Satisfies, errors.IsNotFound)
	c.Assert(results[0].Error, gc.ErrorMatches, `disk volume-42 not found`)
}

func (s *storageSuite) TestDescribeVolumesLegacy(c *gc.C) {
	blob0 := &azuretesting.MockStorageBlob{
		Name_: "volume-0.vhd",
		Properties_: azurestorage.BlobProperties{
			ContentLength: 1024 * 1024 * 1024 * 1024, // 1TiB
		},
	}
	blob1 := &azuretesting.MockStorageBlob{
		Name_: "volume-1.vhd",
		Properties_: azurestorage.BlobProperties{
			ContentLength: 1024 * 1024, // 1MiB
		},
	}
	s.datavhdsContainer.Blobs_ = []internalazurestorage.Blob{blob1, blob0}

	volumeSource := s.volumeSource(c, true)
	results, err := volumeSource.DescribeVolumes([]string{"volume-0", "volume-1", "volume-0", "volume-42"})
	c.Assert(err, jc.ErrorIsNil)
	s.storageClient.CheckCallNames(c, "NewClient", "GetContainerReference")
	s.storageClient.CheckCall(
		c, 0, "NewClient", storageAccountName, fakeStorageAccountKey,
		"storage.azurestack.local", azurestorage.DefaultAPIVersion, true,
	)
	c.Assert(results, gc.HasLen, 4)
	c.Assert(results[:3], jc.DeepEquals, []storage.DescribeVolumesResult{{
		VolumeInfo: &storage.VolumeInfo{
			VolumeId:   "volume-0",
			Size:       1024 * 1024,
			Persistent: true,
		},
	}, {
		VolumeInfo: &storage.VolumeInfo{
			VolumeId:   "volume-1",
			Size:       1,
			Persistent: true,
		},
	}, {
		VolumeInfo: &storage.VolumeInfo{
			VolumeId:   "volume-0",
			Size:       1024 * 1024,
			Persistent: true,
		},
	}})
	c.Assert(results[3].Error, gc.ErrorMatches, "volume-42 not found")
}

func (s *storageSuite) TestDestroyVolumes(c *gc.C) {
	volumeSource := s.volumeSource(c, false)

	volume0Sender := azuretesting.NewSenderWithValue(&autorestazure.ServiceError{})
	volume0Sender.PathPattern = `.*/Microsoft\.Compute/disks/volume-0`
	s.sender = azuretesting.Senders{volume0Sender}

	results, err := volumeSource.DestroyVolumes([]string{"volume-0"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0], jc.ErrorIsNil)
}

func (s *storageSuite) TestDestroyVolumesNotFound(c *gc.C) {
	volumeSource := s.volumeSource(c, false)

	volume42Sender := mocks.NewSender()
	volume42Sender.AppendResponse(mocks.NewResponseWithStatus(
		"disk not found", http.StatusNotFound,
	))
	s.sender = azuretesting.Senders{volume42Sender}

	results, err := volumeSource.DestroyVolumes([]string{"volume-42"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0], jc.ErrorIsNil)
}

func (s *storageSuite) TestDestroyVolumesLegacy(c *gc.C) {
	blob0 := &azuretesting.MockStorageBlob{
		Name_: "volume-0.vhd",
	}
	blob1 := &azuretesting.MockStorageBlob{
		Name_: "volume-42.vhd",
	}
	s.datavhdsContainer.Blobs_ = []internalazurestorage.Blob{blob0, blob1}

	volumeSource := s.volumeSource(c, true)
	results, err := volumeSource.DestroyVolumes([]string{"volume-0", "volume-42"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 2)
	c.Assert(results[0], jc.ErrorIsNil)
	c.Assert(results[1], jc.ErrorIsNil)
	s.storageClient.CheckCallNames(c, "NewClient", "GetContainerReference")
	s.storageClient.CheckCall(c, 1, "GetContainerReference", "datavhds")
	s.datavhdsContainer.CheckCallNames(c, "Blob", "Blob")
	blob0.CheckCallNames(c, "DeleteIfExists")
	blob1.CheckCallNames(c, "DeleteIfExists")
}

func (s *storageSuite) TestAttachVolumes(c *gc.C) {
	s.testAttachVolumes(c, false)
}

func (s *storageSuite) TestAttachVolumesLegacy(c *gc.C) {
	s.testAttachVolumes(c, true)
}

func (s *storageSuite) testAttachVolumes(c *gc.C, legacy bool) {
	// machine-1 has a single data disk with LUN 0.
	machine1DataDisks := []compute.DataDisk{{
		Lun:  to.Int32Ptr(0),
		Name: to.StringPtr("volume-1"),
		Vhd: &compute.VirtualHardDisk{
			URI: to.StringPtr(fmt.Sprintf(
				"https://%s.blob.storage.azurestack.local/datavhds/volume-1.vhd",
				storageAccountName,
			)),
		},
	}}
	// machine-2 has 32 data disks; no LUNs free.
	machine2DataDisks := make([]compute.DataDisk, 32)
	for i := range machine2DataDisks {
		machine2DataDisks[i].Lun = to.Int32Ptr(int32(i))
		machine2DataDisks[i].Name = to.StringPtr(fmt.Sprintf("volume-%d", i))
		machine2DataDisks[i].Vhd = &compute.VirtualHardDisk{
			URI: to.StringPtr(fmt.Sprintf(
				"https://%s.blob.storage.azurestack.local/datavhds/volume-%d.vhd",
				storageAccountName, i,
			)),
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

	virtualMachines := []compute.VirtualMachine{{
		Name: to.StringPtr("machine-0"),
		VirtualMachineProperties: &compute.VirtualMachineProperties{
			StorageProfile: &compute.StorageProfile{},
		},
	}, {
		Name: to.StringPtr("machine-1"),
		VirtualMachineProperties: &compute.VirtualMachineProperties{
			StorageProfile: &compute.StorageProfile{DataDisks: &machine1DataDisks},
		},
	}, {
		Name: to.StringPtr("machine-2"),
		VirtualMachineProperties: &compute.VirtualMachineProperties{
			StorageProfile: &compute.StorageProfile{DataDisks: &machine2DataDisks},
		},
	}}

	// There should be a one API calls to list VMs, and one update per modified instance.
	virtualMachinesSender := azuretesting.NewSenderWithValue(compute.VirtualMachineListResult{
		Value: &virtualMachines,
	})
	virtualMachinesSender.PathPattern = `.*/Microsoft\.Compute/virtualMachines`
	updateVirtualMachine0Sender := azuretesting.NewSenderWithValue(&compute.VirtualMachine{})
	updateVirtualMachine0Sender.PathPattern = `.*/Microsoft\.Compute/virtualMachines/machine-0`

	volumeSource := s.volumeSource(c, legacy)
	s.requests = nil
	s.sender = azuretesting.Senders{
		virtualMachinesSender,
		updateVirtualMachine0Sender,
	}

	results, err := volumeSource.AttachVolumes(params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, len(params))

	c.Check(results[0].Error, jc.ErrorIsNil)
	c.Check(results[1].Error, jc.ErrorIsNil)
	c.Check(results[2].Error, jc.ErrorIsNil)
	c.Check(results[3].Error, gc.ErrorMatches, "instance machine-42 not found")
	c.Check(results[4].Error, gc.ErrorMatches, "choosing LUN: all LUNs are in use")

	// Validate HTTP request bodies.
	c.Assert(s.requests, gc.HasLen, 2)
	c.Assert(s.requests[0].Method, gc.Equals, "GET") // list virtual machines
	c.Assert(s.requests[1].Method, gc.Equals, "PUT") // update machine-0

	makeVhd := func(volumeName string) *compute.VirtualHardDisk {
		if !legacy {
			return nil
		}
		return &compute.VirtualHardDisk{URI: to.StringPtr(fmt.Sprintf(
			"https://%s.blob.storage.azurestack.local/datavhds/%s.vhd",
			storageAccountName, volumeName,
		))}
	}
	makeManagedDisk := func(volumeName string) *compute.ManagedDiskParameters {
		if legacy {
			return nil
		}
		return &compute.ManagedDiskParameters{
			ID: to.StringPtr("/subscriptions/22222222-2222-2222-2222-222222222222/resourceGroups/juju-testenv-model-deadbeef-0bad-400d-8000-4b1d0d06f00d/providers/Microsoft.Compute/disks/" + volumeName),
		}
	}

	machine0DataDisks := []compute.DataDisk{{
		Lun:          to.Int32Ptr(0),
		Name:         to.StringPtr("volume-0"),
		Vhd:          makeVhd("volume-0"),
		ManagedDisk:  makeManagedDisk("volume-0"),
		Caching:      compute.ReadWrite,
		CreateOption: compute.Attach,
	}, {
		Lun:          to.Int32Ptr(1),
		Name:         to.StringPtr("volume-2"),
		Vhd:          makeVhd("volume-2"),
		ManagedDisk:  makeManagedDisk("volume-2"),
		Caching:      compute.ReadWrite,
		CreateOption: compute.Attach,
	}}

	virtualMachines[0].StorageProfile.DataDisks = &machine0DataDisks
	assertRequestBody(c, s.requests[1], &virtualMachines[0])
}

func (s *storageSuite) TestDetachVolumes(c *gc.C) {
	s.testDetachVolumes(c, false)
}

func (s *storageSuite) TestDetachVolumesLegacy(c *gc.C) {
	s.testDetachVolumes(c, true)
}

func (s *storageSuite) testDetachVolumes(c *gc.C, legacy bool) {
	// machine-0 has a three data disks: volume-0, volume-1 and volume-2
	machine0DataDisks := []compute.DataDisk{{
		Lun:  to.Int32Ptr(0),
		Name: to.StringPtr("volume-0"),
	}, {
		Lun:  to.Int32Ptr(1),
		Name: to.StringPtr("volume-1"),
	}, {
		Lun:  to.Int32Ptr(2),
		Name: to.StringPtr("volume-2"),
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

	virtualMachines := []compute.VirtualMachine{{
		Name: to.StringPtr("machine-0"),
		VirtualMachineProperties: &compute.VirtualMachineProperties{
			StorageProfile: &compute.StorageProfile{DataDisks: &machine0DataDisks},
		},
	}, {
		Name: to.StringPtr("machine-1"),
		VirtualMachineProperties: &compute.VirtualMachineProperties{
			StorageProfile: &compute.StorageProfile{},
		},
	}}

	// There should be a one API calls to list VMs, and one update per modified instance.
	virtualMachinesSender := azuretesting.NewSenderWithValue(compute.VirtualMachineListResult{
		Value: &virtualMachines,
	})
	virtualMachinesSender.PathPattern = `.*/Microsoft\.Compute/virtualMachines`
	updateVirtualMachine0Sender := azuretesting.NewSenderWithValue(&compute.VirtualMachine{})
	updateVirtualMachine0Sender.PathPattern = `.*/Microsoft\.Compute/virtualMachines/machine-0`

	volumeSource := s.volumeSource(c, legacy)
	s.requests = nil
	s.sender = azuretesting.Senders{
		virtualMachinesSender,
		updateVirtualMachine0Sender,
	}

	results, err := volumeSource.DetachVolumes(params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, len(params))

	c.Check(results[0], jc.ErrorIsNil)
	c.Check(results[1], jc.ErrorIsNil)
	c.Check(results[2], jc.ErrorIsNil)
	c.Check(results[3], gc.ErrorMatches, "instance machine-42 not found")

	// Validate HTTP request bodies.
	c.Assert(s.requests, gc.HasLen, 2)
	c.Assert(s.requests[0].Method, gc.Equals, "GET") // list virtual machines
	c.Assert(s.requests[1].Method, gc.Equals, "PUT") // update machine-0

	machine0DataDisks = []compute.DataDisk{
		machine0DataDisks[0],
		machine0DataDisks[2],
	}
	virtualMachines[0].StorageProfile.DataDisks = &machine0DataDisks
	assertRequestBody(c, s.requests[1], &virtualMachines[0])
}

func (s *storageSuite) TestDetachVolumesFinal(c *gc.C) {
	// machine-0 has a one data disk: volume-0.
	machine0DataDisks := []compute.DataDisk{{
		Lun:  to.Int32Ptr(0),
		Name: to.StringPtr("volume-0"),
		Vhd: &compute.VirtualHardDisk{
			URI: to.StringPtr(fmt.Sprintf(
				"https://%s.blob.storage.azurestack.local/datavhds/volume-0.vhd",
				storageAccountName,
			)),
		},
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

	virtualMachines := []compute.VirtualMachine{{
		Name: to.StringPtr("machine-0"),
		VirtualMachineProperties: &compute.VirtualMachineProperties{
			StorageProfile: &compute.StorageProfile{DataDisks: &machine0DataDisks},
		},
	}}

	// There should be a one API call to list VMs, and one update to the VM.
	virtualMachinesSender := azuretesting.NewSenderWithValue(compute.VirtualMachineListResult{
		Value: &virtualMachines,
	})
	virtualMachinesSender.PathPattern = `.*/Microsoft\.Compute/virtualMachines`
	updateVirtualMachine0Sender := azuretesting.NewSenderWithValue(&compute.VirtualMachine{})
	updateVirtualMachine0Sender.PathPattern = `.*/Microsoft\.Compute/virtualMachines/machine-0`

	volumeSource := s.volumeSource(c, false)
	s.requests = nil
	s.sender = azuretesting.Senders{
		virtualMachinesSender,
		updateVirtualMachine0Sender,
	}

	results, err := volumeSource.DetachVolumes(params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, len(params))
	c.Assert(results[0], jc.ErrorIsNil)

	// Validate HTTP request bodies.
	c.Assert(s.requests, gc.HasLen, 2)
	c.Assert(s.requests[0].Method, gc.Equals, "GET") // list virtual machines
	c.Assert(s.requests[1].Method, gc.Equals, "PUT") // update machine-0

	machine0DataDisks = []compute.DataDisk{}
	virtualMachines[0].StorageProfile.DataDisks = &machine0DataDisks
	assertRequestBody(c, s.requests[1], &virtualMachines[0])
}
