// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build linux

package kvm

import (
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"

	"github.com/juju/errors"
)

// Run the command as user libvirt-qemu and return the combined output.
// If dir is non-empty, use it as the working directory.
func runAsLibvirt(dir, command string, args ...string) (string, error) {
	uid, gid, err := getUserUIDGID(libvirtUser)
	if err != nil {
		return "", errors.Trace(err)
	}

	cmd := exec.Command(command, args...)
	if dir != "" {
		cmd.Dir = dir
	}

	if dir == "" {
		dir, _ = os.Getwd()
	}
	logger.Debugf("running: %s %v from %s", command, args, dir)
	logger.Debugf("running as uid: %d, gid: %d\n", uid, gid)

	cmd.SysProcAttr = &syscall.SysProcAttr{}
	cmd.SysProcAttr.Credential = &syscall.Credential{
		Uid: uint32(uid),
		Gid: uint32(gid),
	}

	out, err := cmd.CombinedOutput()
	output := string(out)
	logger.Debugf("output: %v", output)

	return output, err

}

// getUserUIDGID returns integer vals for uid and gid for the user. It returns
// -1 when there's an error so no one accidentally thinks 0 is the appropriate
// uid/gid when there's an error.
func getUserUIDGID(_ string) (int, int, error) {
	u, err := user.Lookup(libvirtUser)
	if err != nil {
		return -1, -1, errors.Trace(err)
	}
	uid, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil {
		return -1, -1, errors.Trace(err)
	}
	gid, err := strconv.ParseUint(u.Gid, 10, 32)
	if err != nil {
		return -1, -1, errors.Trace(err)
	}
	return int(uid), int(gid), nil
}
