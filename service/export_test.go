// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"io/ioutil"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/version"
)

type patcher interface {
	PatchValue(interface{}, interface{})
}

func PatchVersion(patcher patcher, vers version.Binary) {
	patcher.PatchValue(&jujuVersion, vers)
}

func PatchGOOS(patcher patcher, os string) {
	patcher.PatchValue(&runtimeOS, os)
}

func PatchPid1File(c *gc.C, patcher patcher, executable string) {
	dirname := c.MkDir()
	filename := filepath.Join(dirname, "pid1cmdline")
	err := ioutil.WriteFile(filename, []byte(executable), 0644)
	c.Assert(err, jc.ErrorIsNil)

	patcher.PatchValue(&pid1File, filename)
}
