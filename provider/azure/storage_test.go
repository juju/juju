// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure_test

import (
	"fmt"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/Godeps/_workspace/src/github.com/Azure/go-autorest/autorest/to"
	"github.com/Azure/azure-sdk-for-go/arm/compute"
	"github.com/Azure/azure-sdk-for-go/arm/network"
	azurestorage "github.com/Azure/azure-sdk-for-go/storage"
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/azure"
	"github.com/juju/juju/provider/azure/internal/azuretesting"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
)

type storageSuite struct {
	testing.BaseSuite

	storageClient azuretesting.MockStorageClient
	provider      storage.Provider
	requests      []*http.Request
	sender        azuretesting.Senders
}

var _ = gc.Suite(&storageSuite{})

func (s *storageSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.storageClient = azuretesting.MockStorageClient{}
	s.requests = nil
	_, s.provider = newProviders(c, azure.ProviderConfig{
		Sender:           &s.sender,
		NewStorageClient: s.storageClient.NewClient,
		RequestInspector: requestRecorder(&s.requests),
	})
	s.sender = nil
}

func (s *storageSuite) volumeSource(c *gc.C, attrs ...testing.Attrs) storage.VolumeSource {
	storageConfig, err := storage.NewConfig("azure", "azure", nil)
	c.Assert(err, jc.ErrorIsNil)

	attrs = append([]testing.Attrs{{
		"storage-account":     fakeStorageAccount,
		"storage-account-key": fakeStorageAccountKey,
	}}, attrs...)
	cfg := makeTestModelConfig(c, attrs...)
	volumeSource, err := s.provider.VolumeSource(cfg, storageConfig)
	c.Assert(err, jc.ErrorIsNil)

	// Force an explicit refresh of the access token, so it isn't done
	// implicitly during the tests.
	s.sender = azuretesting.Senders{tokenRefreshSender()}
	err = azure.ForceVolumeSourceTokenRefresh(volumeSource)
	c.Assert(err, jc.ErrorIsNil)
	return volumeSource
}

func (s *storageSuite) TestVolumeSource(c *gc.C) {
	vs := s.volumeSource(c)
	c.Assert(vs, gc.NotNil)
}

func (s *storageSuite) TestFilesystemSource(c *gc.C) {
	storageConfig, err := storage.NewConfig("azure", "azure", nil)
	c.Assert(err, jc.ErrorIsNil)

	cfg := makeTestModelConfig(c)
	_, err = s.provider.FilesystemSource(cfg, storageConfig)
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
	// machine-1 has a single data disk with LUN 0.
	machine1DataDisks := []compute.DataDisk{{Lun: to.IntPtr(0)}}
	// machine-2 has 32 data disks; no LUNs free.
	machine2DataDisks := make([]compute.DataDisk, 32)
	for i := range machine2DataDisks {
		machine2DataDisks[i].Lun = to.IntPtr(i)
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
		Properties: &compute.VirtualMachineProperties{
			StorageProfile: &compute.StorageProfile{},
		},
	}, {
		Name: to.StringPtr("machine-1"),
		Properties: &compute.VirtualMachineProperties{
			StorageProfile: &compute.StorageProfile{DataDisks: &machine1DataDisks},
		},
	}, {
		Name: to.StringPtr("machine-2"),
		Properties: &compute.VirtualMachineProperties{
			StorageProfile: &compute.StorageProfile{DataDisks: &machine2DataDisks},
		},
	}}

	// There should be a couple of API calls to list instances,
	// and one update per modified instance.
	nics := []network.Interface{
		makeNetworkInterface("nic-0", "machine-0"),
		makeNetworkInterface("nic-1", "machine-1"),
		makeNetworkInterface("nic-2", "machine-2"),
	}
	nicsSender := azuretesting.NewSenderWithValue(network.InterfaceListResult{
		Value: &nics,
	})
	nicsSender.PathPattern = `.*/Microsoft\.Network/networkInterfaces`
	virtualMachinesSender := azuretesting.NewSenderWithValue(compute.VirtualMachineListResult{
		Value: &virtualMachines,
	})
	virtualMachinesSender.PathPattern = `.*/Microsoft\.Compute/virtualMachines`
	updateVirtualMachine0Sender := azuretesting.NewSenderWithValue(&compute.VirtualMachine{})
	updateVirtualMachine0Sender.PathPattern = `.*/Microsoft\.Compute/virtualMachines/machine-0`
	updateVirtualMachine1Sender := azuretesting.NewSenderWithValue(&compute.VirtualMachine{})
	updateVirtualMachine1Sender.PathPattern = `.*/Microsoft\.Compute/virtualMachines/machine-1`
	volumeSource := s.volumeSource(c)
	s.sender = azuretesting.Senders{
		nicsSender,
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

	// Validate HTTP request bodies.
	c.Assert(s.requests, gc.HasLen, 4)
	c.Assert(s.requests[0].Method, gc.Equals, "GET") // list NICs
	c.Assert(s.requests[1].Method, gc.Equals, "GET") // list virtual machines
	c.Assert(s.requests[2].Method, gc.Equals, "PUT") // update machine-0
	c.Assert(s.requests[3].Method, gc.Equals, "PUT") // update machine-1

	machine0DataDisks := []compute.DataDisk{{
		Lun:        to.IntPtr(0),
		DiskSizeGB: to.IntPtr(1),
		Name:       to.StringPtr("volume-0"),
		Vhd: &compute.VirtualHardDisk{URI: to.StringPtr(fmt.Sprintf(
			"https://%s.blob.storage.azurestack.local/datavhds/volume-0.vhd",
			fakeStorageAccount,
		))},
		Caching:      compute.ReadWrite,
		CreateOption: compute.Empty,
	}, {
		Lun:        to.IntPtr(1),
		DiskSizeGB: to.IntPtr(1),
		Name:       to.StringPtr("volume-2"),
		Vhd: &compute.VirtualHardDisk{URI: to.StringPtr(fmt.Sprintf(
			"https://%s.blob.storage.azurestack.local/datavhds/volume-2.vhd",
			fakeStorageAccount,
		))},
		Caching:      compute.ReadWrite,
		CreateOption: compute.Empty,
	}}
	virtualMachines[0].Properties.StorageProfile.DataDisks = &machine0DataDisks
	assertRequestBody(c, s.requests[2], &virtualMachines[0])

	machine1DataDisks = append(machine1DataDisks, compute.DataDisk{
		Lun:        to.IntPtr(1),
		DiskSizeGB: to.IntPtr(2),
		Name:       to.StringPtr("volume-1"),
		Vhd: &compute.VirtualHardDisk{URI: to.StringPtr(fmt.Sprintf(
			"https://%s.blob.storage.azurestack.local/datavhds/volume-1.vhd",
			fakeStorageAccount,
		))},
		Caching:      compute.ReadWrite,
		CreateOption: compute.Empty,
	})
	assertRequestBody(c, s.requests[3], &virtualMachines[1])
}

func (s *storageSuite) TestListVolumes(c *gc.C) {
	s.storageClient.ListBlobsFunc = func(
		container string,
		params azurestorage.ListBlobsParameters,
	) (azurestorage.BlobListResponse, error) {
		return azurestorage.BlobListResponse{
			Blobs: []azurestorage.Blob{{
				Name: "volume-1.vhd",
				Properties: azurestorage.BlobProperties{
					ContentLength: 1024 * 1024, // 1MiB
				},
			}, {
				Name: "volume-0.vhd",
				Properties: azurestorage.BlobProperties{
					ContentLength: 1024 * 1024 * 1024 * 1024, // 1TiB
				},
			}, {
				Name: "junk.vhd",
			}, {
				Name: "volume",
			}},
		}, nil
	}

	volumeSource := s.volumeSource(c)
	volumeIds, err := volumeSource.ListVolumes()
	c.Assert(err, jc.ErrorIsNil)
	s.storageClient.CheckCallNames(c, "NewClient", "ListBlobs")
	s.storageClient.CheckCall(
		c, 0, "NewClient", fakeStorageAccount, fakeStorageAccountKey,
		"storage.azurestack.local", azurestorage.DefaultAPIVersion, true,
	)
	s.storageClient.CheckCall(c, 1, "ListBlobs", "datavhds", azurestorage.ListBlobsParameters{})
	c.Assert(volumeIds, jc.DeepEquals, []string{"volume-1", "volume-0"})
}

func (s *storageSuite) TestListVolumesErrors(c *gc.C) {
	volumeSource := s.volumeSource(c)
	s.storageClient.SetErrors(errors.New("no client for you"))
	_, err := volumeSource.ListVolumes()
	c.Assert(err, gc.ErrorMatches, "listing volumes: getting storage client: no client for you")

	s.storageClient.SetErrors(nil, errors.New("no blobs for you"))
	_, err = volumeSource.ListVolumes()
	c.Assert(err, gc.ErrorMatches, "listing volumes: listing blobs: no blobs for you")
}

func (s *storageSuite) TestDescribeVolumes(c *gc.C) {
	s.storageClient.ListBlobsFunc = func(
		container string,
		params azurestorage.ListBlobsParameters,
	) (azurestorage.BlobListResponse, error) {
		return azurestorage.BlobListResponse{
			Blobs: []azurestorage.Blob{{
				Name: "volume-1.vhd",
				Properties: azurestorage.BlobProperties{
					ContentLength: 1024 * 1024, // 1MiB
				},
			}, {
				Name: "volume-0.vhd",
				Properties: azurestorage.BlobProperties{
					ContentLength: 1024 * 1024 * 1024 * 1024, // 1TiB
				},
			}},
		}, nil
	}

	volumeSource := s.volumeSource(c)
	results, err := volumeSource.DescribeVolumes([]string{"volume-0", "volume-1", "volume-0", "volume-42"})
	c.Assert(err, jc.ErrorIsNil)
	s.storageClient.CheckCallNames(c, "NewClient", "ListBlobs")
	s.storageClient.CheckCall(
		c, 0, "NewClient", fakeStorageAccount, fakeStorageAccountKey,
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
	volumeSource := s.volumeSource(c)
	results, err := volumeSource.DestroyVolumes([]string{"volume-0", "volume-42"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 2)
	c.Assert(results[0], jc.ErrorIsNil)
	c.Assert(results[1], jc.ErrorIsNil)
	s.storageClient.CheckCallNames(c, "NewClient", "DeleteBlobIfExists", "DeleteBlobIfExists")
	s.storageClient.CheckCall(c, 1, "DeleteBlobIfExists", "datavhds", "volume-0.vhd")
	s.storageClient.CheckCall(c, 2, "DeleteBlobIfExists", "datavhds", "volume-42.vhd")
}

func (s *storageSuite) TestAttachVolumes(c *gc.C) {
	// machine-1 has a single data disk with LUN 0.
	machine1DataDisks := []compute.DataDisk{{
		Lun:  to.IntPtr(0),
		Name: to.StringPtr("volume-1"),
		Vhd: &compute.VirtualHardDisk{
			URI: to.StringPtr(fmt.Sprintf(
				"https://%s.blob.storage.azurestack.local/datavhds/volume-1.vhd",
				fakeStorageAccount,
			)),
		},
	}}
	// machine-2 has 32 data disks; no LUNs free.
	machine2DataDisks := make([]compute.DataDisk, 32)
	for i := range machine2DataDisks {
		machine2DataDisks[i].Lun = to.IntPtr(i)
		machine2DataDisks[i].Name = to.StringPtr(fmt.Sprintf("volume-%d", i))
		machine2DataDisks[i].Vhd = &compute.VirtualHardDisk{
			URI: to.StringPtr(fmt.Sprintf(
				"https://%s.blob.storage.azurestack.local/datavhds/volume-%d.vhd",
				fakeStorageAccount, i,
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
		Properties: &compute.VirtualMachineProperties{
			StorageProfile: &compute.StorageProfile{},
		},
	}, {
		Name: to.StringPtr("machine-1"),
		Properties: &compute.VirtualMachineProperties{
			StorageProfile: &compute.StorageProfile{DataDisks: &machine1DataDisks},
		},
	}, {
		Name: to.StringPtr("machine-2"),
		Properties: &compute.VirtualMachineProperties{
			StorageProfile: &compute.StorageProfile{DataDisks: &machine2DataDisks},
		},
	}}

	// There should be a couple of API calls to list instances,
	// and one update per modified instance.
	nics := []network.Interface{
		makeNetworkInterface("nic-0", "machine-0"),
		makeNetworkInterface("nic-1", "machine-1"),
		makeNetworkInterface("nic-2", "machine-2"),
	}
	nicsSender := azuretesting.NewSenderWithValue(network.InterfaceListResult{
		Value: &nics,
	})
	nicsSender.PathPattern = `.*/Microsoft\.Network/networkInterfaces`
	virtualMachinesSender := azuretesting.NewSenderWithValue(compute.VirtualMachineListResult{
		Value: &virtualMachines,
	})
	virtualMachinesSender.PathPattern = `.*/Microsoft\.Compute/virtualMachines`
	updateVirtualMachine0Sender := azuretesting.NewSenderWithValue(&compute.VirtualMachine{})
	updateVirtualMachine0Sender.PathPattern = `.*/Microsoft\.Compute/virtualMachines/machine-0`
	volumeSource := s.volumeSource(c)
	s.sender = azuretesting.Senders{
		nicsSender,
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
	c.Assert(s.requests, gc.HasLen, 3)
	c.Assert(s.requests[0].Method, gc.Equals, "GET") // list NICs
	c.Assert(s.requests[1].Method, gc.Equals, "GET") // list virtual machines
	c.Assert(s.requests[2].Method, gc.Equals, "PUT") // update machine-0

	machine0DataDisks := []compute.DataDisk{{
		Lun:  to.IntPtr(0),
		Name: to.StringPtr("volume-0"),
		Vhd: &compute.VirtualHardDisk{URI: to.StringPtr(fmt.Sprintf(
			"https://%s.blob.storage.azurestack.local/datavhds/volume-0.vhd",
			fakeStorageAccount,
		))},
		Caching:      compute.ReadWrite,
		CreateOption: compute.Attach,
	}, {
		Lun:  to.IntPtr(1),
		Name: to.StringPtr("volume-2"),
		Vhd: &compute.VirtualHardDisk{URI: to.StringPtr(fmt.Sprintf(
			"https://%s.blob.storage.azurestack.local/datavhds/volume-2.vhd",
			fakeStorageAccount,
		))},
		Caching:      compute.ReadWrite,
		CreateOption: compute.Attach,
	}}
	virtualMachines[0].Properties.StorageProfile.DataDisks = &machine0DataDisks
	assertRequestBody(c, s.requests[2], &virtualMachines[0])
}

func (s *storageSuite) TestDetachVolumes(c *gc.C) {
	// machine-0 has a three data disks: volume-0, volume-1 and volume-2
	machine0DataDisks := []compute.DataDisk{{
		Lun:  to.IntPtr(0),
		Name: to.StringPtr("volume-0"),
		Vhd: &compute.VirtualHardDisk{
			URI: to.StringPtr(fmt.Sprintf(
				"https://%s.blob.storage.azurestack.local/datavhds/volume-0.vhd",
				fakeStorageAccount,
			)),
		},
	}, {
		Lun:  to.IntPtr(1),
		Name: to.StringPtr("volume-1"),
		Vhd: &compute.VirtualHardDisk{
			URI: to.StringPtr(fmt.Sprintf(
				"https://%s.blob.storage.azurestack.local/datavhds/volume-1.vhd",
				fakeStorageAccount,
			)),
		},
	}, {
		Lun:  to.IntPtr(2),
		Name: to.StringPtr("volume-2"),
		Vhd: &compute.VirtualHardDisk{
			URI: to.StringPtr(fmt.Sprintf(
				"https://%s.blob.storage.azurestack.local/datavhds/volume-2.vhd",
				fakeStorageAccount,
			)),
		},
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
		Properties: &compute.VirtualMachineProperties{
			StorageProfile: &compute.StorageProfile{DataDisks: &machine0DataDisks},
		},
	}, {
		Name: to.StringPtr("machine-1"),
		Properties: &compute.VirtualMachineProperties{
			StorageProfile: &compute.StorageProfile{},
		},
	}}

	// There should be a couple of API calls to list instances,
	// and one update per modified instance.
	nics := []network.Interface{
		makeNetworkInterface("nic-0", "machine-0"),
		makeNetworkInterface("nic-1", "machine-1"),
	}
	nicsSender := azuretesting.NewSenderWithValue(network.InterfaceListResult{
		Value: &nics,
	})
	nicsSender.PathPattern = `.*/Microsoft\.Network/networkInterfaces`
	virtualMachinesSender := azuretesting.NewSenderWithValue(compute.VirtualMachineListResult{
		Value: &virtualMachines,
	})
	virtualMachinesSender.PathPattern = `.*/Microsoft\.Compute/virtualMachines`
	updateVirtualMachine0Sender := azuretesting.NewSenderWithValue(&compute.VirtualMachine{})
	updateVirtualMachine0Sender.PathPattern = `.*/Microsoft\.Compute/virtualMachines/machine-0`
	volumeSource := s.volumeSource(c)
	s.sender = azuretesting.Senders{
		nicsSender,
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
	c.Assert(s.requests, gc.HasLen, 3)
	c.Assert(s.requests[0].Method, gc.Equals, "GET") // list NICs
	c.Assert(s.requests[1].Method, gc.Equals, "GET") // list virtual machines
	c.Assert(s.requests[2].Method, gc.Equals, "PUT") // update machine-0

	machine0DataDisks = []compute.DataDisk{
		machine0DataDisks[0],
		machine0DataDisks[2],
	}
	virtualMachines[0].Properties.StorageProfile.DataDisks = &machine0DataDisks
	assertRequestBody(c, s.requests[2], &virtualMachines[0])
}
