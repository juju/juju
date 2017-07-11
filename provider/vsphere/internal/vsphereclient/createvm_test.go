// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphereclient

import (
	"io/ioutil"
	"net/http"
	"path/filepath"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	"golang.org/x/net/context"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/constraints"
	coretesting "github.com/juju/juju/testing"
)

func (s *clientSuite) TestCreateVirtualMachine(c *gc.C) {
	var statusUpdates []string
	statusUpdatesCh := make(chan string, 3)
	dequeueStatusUpdates := func() {
		for {
			select {
			case <-statusUpdatesCh:
			default:
				return
			}
		}
	}

	args := baseCreateVirtualMachineParams(c)
	testClock := args.Clock.(*testing.Clock)
	s.onImageUpload = func(r *http.Request) {
		dequeueStatusUpdates()

		// Wait 1.5 seconds, which is long enough to trigger the status
		// update timer, but not the lease update timer.
		testClock.WaitAdvance(1500*time.Millisecond, coretesting.LongWait, 2)
		// Waiting for the status update here guarantees that a report is
		// available, since we don't update status until that is true.
		<-statusUpdatesCh

		// Now wait 0.5 seconds, which is long enough to trigger the
		// lease updater's timer, but not the status updater's timer.
		// Since the status update was received above, we know that a
		// report has been delivered and so the lease updater should
		// report 100%.
		testClock.WaitAdvance(500*time.Millisecond, coretesting.LongWait, 2)
		<-s.roundTripper.leaseProgress
		s.onImageUpload = nil
	}
	args.UpdateProgress = func(status string) {
		statusUpdatesCh <- status
		statusUpdates = append(statusUpdates, status)
	}

	client := s.newFakeClient(&s.roundTripper, "dc0")
	_, err := client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusUpdates, jc.DeepEquals, []string{
		"creating import spec",
		`creating VM "vm-0"`,
		"uploading ubuntu-14.04-server-cloudimg-amd64.vmdk: 100.00% (0B/s)",
		"powering on",
	})

	c.Assert(s.uploadRequests, gc.HasLen, 1)
	contents, err := ioutil.ReadAll(s.uploadRequests[0].Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(contents), gc.Equals, "image-contents")

	s.roundTripper.CheckCalls(c, []testing.StubCall{
		retrievePropertiesStubCall("FakeDatastore1", "FakeDatastore2"),
		testing.StubCall{"CreateImportSpec", []interface{}{
			"ovf-descriptor",
			types.ManagedObjectReference{Type: "Datastore", Value: "FakeDatastore2"},
		}},
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeDatacenter"),
		retrievePropertiesStubCall("FakeRootFolder"),
		retrievePropertiesStubCall("FakeDatacenter"),
		retrievePropertiesStubCall("FakeVmFolder"),
		retrievePropertiesStubCall("FakeHostFolder"),
		testing.StubCall{"ImportVApp", nil},
		testing.StubCall{"CreatePropertyCollector", nil},
		testing.StubCall{"CreateFilter", nil},
		testing.StubCall{"WaitForUpdatesEx", nil},
		testing.StubCall{"HttpNfcLeaseProgress", []interface{}{"FakeLease", int32(100)}},
		testing.StubCall{"HttpNfcLeaseComplete", []interface{}{"FakeLease"}},
		testing.StubCall{"PowerOnVM_Task", nil},
		testing.StubCall{"CreatePropertyCollector", nil},
		testing.StubCall{"CreateFilter", nil},
		testing.StubCall{"WaitForUpdatesEx", nil},
		retrievePropertiesStubCall(""),
	})
}

func (s *clientSuite) TestCreateVirtualMachineDatastoreSpecified(c *gc.C) {
	args := baseCreateVirtualMachineParams(c)
	args.Datastore = "datastore1"
	args.ComputeResource.Datastore = []types.ManagedObjectReference{{
		Type:  "Datastore",
		Value: "FakeDatastore2",
	}, {
		Type:  "Datastore",
		Value: "FakeDatastore1",
	}}

	client := s.newFakeClient(&s.roundTripper, "dc0")
	_, err := client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	s.roundTripper.CheckCall(
		c, 1, "CreateImportSpec", "ovf-descriptor",
		types.ManagedObjectReference{Type: "Datastore", Value: "FakeDatastore1"},
	)
}

func (s *clientSuite) TestCreateVirtualMachineDatastoreNotFound(c *gc.C) {
	args := baseCreateVirtualMachineParams(c)
	args.Datastore = "datastore3"

	client := s.newFakeClient(&s.roundTripper, "dc0")
	_, err := client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, gc.ErrorMatches, `creating import spec: could not find datastore "datastore3"`)
}

func (s *clientSuite) TestCreateVirtualMachineDatastoreNoneAccessible(c *gc.C) {
	args := baseCreateVirtualMachineParams(c)
	args.ComputeResource.Datastore = []types.ManagedObjectReference{{
		Type:  "Datastore",
		Value: "FakeDatastore1",
	}}

	client := s.newFakeClient(&s.roundTripper, "dc0")
	_, err := client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, gc.ErrorMatches, "creating import spec: could not find an accessible datastore")
}

func baseCreateVirtualMachineParams(c *gc.C) CreateVirtualMachineParams {
	ovaDir := makeOvaDir(c)
	return CreateVirtualMachineParams{
		Name:     "vm-0",
		Folder:   "foo",
		OVADir:   ovaDir,
		OVF:      "ovf-descriptor",
		UserData: "baz",
		ComputeResource: &mo.ComputeResource{
			ResourcePool: &types.ManagedObjectReference{
				Type:  "ResourcePool",
				Value: "FakeResourcePool1",
			},
			Datastore: []types.ManagedObjectReference{{
				Type:  "Datastore",
				Value: "FakeDatastore1",
			}, {
				Type:  "Datastore",
				Value: "FakeDatastore2",
			}},
		},
		Metadata:               map[string]string{"k": "v"},
		Constraints:            constraints.Value{},
		ExternalNetwork:        "arpa",
		UpdateProgress:         func(status string) {},
		UpdateProgressInterval: time.Second,
		Clock: testing.NewClock(time.Time{}),
	}
}

func makeOvaDir(c *gc.C) string {
	ovaDir := c.MkDir()
	err := ioutil.WriteFile(
		filepath.Join(ovaDir, "ubuntu-14.04-server-cloudimg-amd64.vmdk"),
		[]byte("image-contents"),
		0644,
	)
	c.Assert(err, jc.ErrorIsNil)
	return ovaDir
}
