// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package looputil

import "os"

func NewTestLoopDeviceManager(
	run func(cmd string, args ...string) (string, error),
	stat func(path string) (os.FileInfo, error),
	inode func(info os.FileInfo) uint64,
) LoopDeviceManager {
	return &loopDeviceManager{run, stat, inode}
}
