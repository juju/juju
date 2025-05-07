// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
//go:build !windows

package wrench_test

import (
	"os"
	"syscall"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/wrench"
)

const fileNotFound = `stat .+: no such file or directory`

// Patch out the os.Stat call used by wrench so that a particular file
// appears to be owned by a UID that isn't Juju's UID.
func (s *wrenchSuite) tweakOwner(c *tc.C, targetPath string) {
	s.PatchValue(wrench.Stat, func(path string) (fi os.FileInfo, err error) {
		fi, err = os.Stat(path)
		if err != nil {
			return
		}
		if path == targetPath {
			statStruct, ok := fi.Sys().(*syscall.Stat_t)
			if !ok {
				c.Skip("this test only supports POSIX systems")
			}
			statStruct.Uid = notJujuUid
		}
		return
	})
}
