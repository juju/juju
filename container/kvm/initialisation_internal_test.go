// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type initialisationInternalSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&initialisationInternalSuite{})

func (initialisationInternalSuite) TestCreatePool(c *gc.C) {
	tmpDir, err := ioutil.TempDir("", "juju-initialisationInternalSuite")
	defer func() {
		err = os.RemoveAll(tmpDir)
		if err != nil {
			c.Errorf("failed to remove tmpDir: %s\n", err)
		}
	}()
	c.Check(err, jc.ErrorIsNil)
	pathfinder := func(s string) (string, error) {
		return tmpDir, nil
	}
	stub := runStub{}
	chown := func(string) error { return nil }
	err = ensurePool(nil, pathfinder, stub.Run, chown)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(stub.Calls(), jc.DeepEquals, []string{
		fmt.Sprintf(" virsh pool-define-as juju-pool dir - - - - %s/kvm/guests", tmpDir),
		" virsh pool-build juju-pool",
		" virsh pool-start juju-pool",
		" virsh pool-autostart juju-pool",
	})
}

func (initialisationInternalSuite) TestStartPool(c *gc.C) {
	tmpDir, err := ioutil.TempDir("", "juju-initialisationInternalSuite")
	defer func() {
		err = os.RemoveAll(tmpDir)
		if err != nil {
			c.Errorf("failed to remove tmpDir: %s\n", err)
		}
	}()
	c.Check(err, jc.ErrorIsNil)
	pathfinder := func(s string) (string, error) {
		return tmpDir, nil
	}
	poolInfo := &libvirtPool{Name: "juju-pool", Autostart: "no", State: "inactive"}
	stub := runStub{}
	chown := func(string) error { return nil }
	err = ensurePool(poolInfo, pathfinder, stub.Run, chown)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(stub.Calls(), jc.DeepEquals, []string{
		" virsh pool-build juju-pool",
		" virsh pool-start juju-pool",
		" virsh pool-autostart juju-pool",
	})
}

func (initialisationInternalSuite) TestAutoStartPool(c *gc.C) {
	tmpDir, err := ioutil.TempDir("", "juju-initialisationInternalSuite")
	defer func() {
		err = os.RemoveAll(tmpDir)
		if err != nil {
			c.Errorf("failed to remove tmpDir: %s\n", err)
		}
	}()
	c.Check(err, jc.ErrorIsNil)
	pathfinder := func(s string) (string, error) {
		return tmpDir, nil
	}
	poolInfo := &libvirtPool{Name: "juju-pool", Autostart: "no", State: "running"}
	stub := runStub{}
	chown := func(string) error { return nil }
	err = ensurePool(poolInfo, pathfinder, stub.Run, chown)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(stub.Calls(), jc.DeepEquals, []string{
		" virsh pool-autostart juju-pool",
	})
}

func (initialisationInternalSuite) TestRequiredPackagesAMD64(c *gc.C) {
	got := getRequiredPackages("amd64")
	c.Assert(got, jc.DeepEquals, []string{"qemu-kvm", "qemu-utils", "genisoimage", "libvirt-bin"})
}

func (initialisationInternalSuite) TestRequiredPackagesARM64(c *gc.C) {
	got := getRequiredPackages("arm64")
	c.Assert(got, jc.DeepEquals, []string{"qemu-efi", "qemu-kvm", "qemu-utils", "genisoimage", "libvirt-bin"})
}
