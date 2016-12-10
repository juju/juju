// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build linux
// +build amd64 arm64 ppc64el

package kvm

import (
	"fmt"
	"io/ioutil"
	"os"

	gc "gopkg.in/check.v1"

	jc "github.com/juju/testing/checkers"
)

// gocheck boilerplate.
type initialisationInternalSuite struct{}

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
	err = createPool(pathfinder, stub.Run, chown)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(stub.Calls(), jc.DeepEquals, []string{
		fmt.Sprintf("virsh pool-define-as juju-pool dir - - - - %s/kvm/guests", tmpDir),
		"virsh pool-build juju-pool",
		"virsh pool-start juju-pool",
		"virsh pool-autostart juju-pool",
	})
}
