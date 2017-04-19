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
	ovaDir := c.MkDir()
	err := ioutil.WriteFile(
		filepath.Join(ovaDir, "ubuntu-14.04-server-cloudimg-amd64.vmdk"),
		[]byte("image-contents"),
		0644,
	)
	c.Assert(err, jc.ErrorIsNil)

	testClock := testing.NewClock(time.Time{})
	s.onImageUpload = func(*http.Request) {
		// Wait until the status and lease updaters are waiting for
		// the time to tick over, and then advance by 2 seconds to
		// wake them both up.
		testClock.WaitAdvance(2*time.Second, coretesting.LongWait, 2)
		s.onImageUpload = nil
	}

	var progressUpdates []string
	client := s.newFakeClient(&s.roundTripper, "dc0")
	args := CreateVirtualMachineParams{
		Name:     "vm-0",
		Folder:   "foo",
		OVADir:   ovaDir,
		OVF:      "bar",
		UserData: "baz",
		ComputeResource: &mo.ComputeResource{
			ResourcePool: &types.ManagedObjectReference{
				Type:  "ResourcePool",
				Value: "FakeResourcePool1",
			},
			Datastore: []types.ManagedObjectReference{{
				Type:  "Datastore",
				Value: "FakeDatastore1",
			}},
		},
		Metadata:        map[string]string{"k": "v"},
		Constraints:     constraints.Value{},
		ExternalNetwork: "arpa",
		UpdateProgress: func(progress string) {
			progressUpdates = append(progressUpdates, progress)
		},
		UpdateProgressInterval: 2 * time.Second,
		Clock: testClock,
	}
	_, err = client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(progressUpdates, jc.DeepEquals, []string{
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
		testing.StubCall{"CreateImportSpec", nil},
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
