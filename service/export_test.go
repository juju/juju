// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"

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

func PatchPid1File(c *gc.C, patcher patcher, executable, verText string) string {
	dirname := c.MkDir()

	exeSuffix := ".sh"
	if runtime.GOOS == "windows" {
		executable = filepath.FromSlash(executable)
		exeSuffix = ".bat"
	}
	exeName := filepath.Join(dirname, executable) + exeSuffix
	if verText != "" {
		err := os.MkdirAll(filepath.Dir(exeName), 0755)
		c.Assert(err, jc.ErrorIsNil)
		err = ioutil.WriteFile(exeName, []byte("#!/usr/bin/env bash\necho "+verText), 0755)
		c.Assert(err, jc.ErrorIsNil)
	}

	filename := filepath.Join(dirname, "pid1cmdline")
	err := ioutil.WriteFile(filename, []byte(exeName), 0644)
	c.Assert(err, jc.ErrorIsNil)

	patcher.PatchValue(&pid1File, filename)
	return exeName
}
