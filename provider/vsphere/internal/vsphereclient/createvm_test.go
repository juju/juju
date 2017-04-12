// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphereclient

import (
	"io/ioutil"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	"golang.org/x/net/context"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/constraints"
)

func (s *clientSuite) TestCreateVirtualMachine(c *gc.C) {
	ovaDir := c.MkDir()
	err := ioutil.WriteFile(
		filepath.Join(ovaDir, "ubuntu-14.04-server-cloudimg-amd64.vmdk"),
		[]byte("image-contents"),
		0644,
	)
	c.Assert(err, jc.ErrorIsNil)

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
		UpdateProgress:  func(string) {},
	}
	_, err = client.CreateVirtualMachine(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.uploadRequests, gc.HasLen, 1)
	contents, err := ioutil.ReadAll(s.uploadRequests[0].Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(contents), gc.Equals, "image-contents")
}
