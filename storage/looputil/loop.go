// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package looputil

import (
	"bytes"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.storage.looputil")

// LoopDeviceManager is an API for dealing with storage loop devices.
type LoopDeviceManager interface {
	// DetachLoopDevices detaches loop devices that are backed by files
	// inside the given root filesystem with the given prefix.
	DetachLoopDevices(rootfs, prefix string) error
}

type runFunc func(cmd string, args ...string) (string, error)

type loopDeviceManager struct {
	run   runFunc
	stat  func(string) (os.FileInfo, error)
	inode func(os.FileInfo) uint64
}

// NewLoopDeviceManager returns a new LoopDeviceManager for dealing
// with storage loop devices on the local machine.
func NewLoopDeviceManager() LoopDeviceManager {
	run := func(cmd string, args ...string) (string, error) {
		out, err := exec.Command(cmd, args...).CombinedOutput()
		out = bytes.TrimSpace(out)
		if err != nil {
			if len(out) > 0 {
				err = errors.Annotatef(err, "failed with %q", out)
			}
			return "", err
		}
		return string(out), nil
	}
	return &loopDeviceManager{run, os.Stat, fileInode}
}

// DetachLoopDevices detaches loop devices that are backed by files
// inside the given root filesystem with the given prefix.
func (m *loopDeviceManager) DetachLoopDevices(rootfs, prefix string) error {
	logger.Debugf("detaching loop devices inside %q", rootfs)
	loopDevices, err := loopDevices(m.run)
	if err != nil {
		return errors.Annotate(err, "listing loop devices")
	}

	for _, info := range loopDevices {
		logger.Debugf("checking loop device: %v", info)
		if !strings.HasPrefix(info.backingFile, prefix) {
			continue
		}
		rootedBackingFile := path.Join(rootfs, info.backingFile)
		st, err := m.stat(rootedBackingFile)
		if os.IsNotExist(err) {
			continue
		} else if err != nil {
			return errors.Annotate(err, "querying backing file")
		}
		if m.inode(st) != info.backingInode {
			continue
		}
		logger.Debugf("detaching loop device %q", info.name)
		if err := detachLoopDevice(m.run, info.name); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

type loopDeviceInfo struct {
	name         string
	backingFile  string
	backingInode uint64
}

func loopDevices(run runFunc) ([]loopDeviceInfo, error) {
	out, err := run("losetup", "-a")
	if err != nil {
		return nil, errors.Trace(err)
	}
	if out == "" {
		return nil, nil
	}
	lines := strings.Split(out, "\n")
	devices := make([]loopDeviceInfo, len(lines))
	for i, line := range lines {
		info, err := parseLoopDeviceInfo(strings.TrimSpace(line))
		if err != nil {
			return nil, errors.Trace(err)
		}
		devices[i] = info
	}
	return devices, nil
}

// e.g. "/dev/loop0: [0021]:7504142 (/tmp/test.dat)"
//      "/dev/loop0: [002f]:7504142 (/tmp/test.dat (deleted))"
var loopDeviceInfoRegexp = regexp.MustCompile(`^([^ ]+): \[[[:xdigit:]]+\]:(\d+) \((.*?)(?: \(.*\))?\)$`)

func parseLoopDeviceInfo(line string) (loopDeviceInfo, error) {
	submatch := loopDeviceInfoRegexp.FindStringSubmatch(line)
	if submatch == nil {
		return loopDeviceInfo{}, errors.Errorf("cannot parse loop device info from %q", line)
	}
	name := submatch[1]
	backingFile := submatch[3]
	backingInode, err := strconv.ParseUint(submatch[2], 10, 64)
	if err != nil {
		return loopDeviceInfo{}, errors.Annotate(err, "parsing inode")
	}
	return loopDeviceInfo{name, backingFile, backingInode}, nil
}

func detachLoopDevice(run runFunc, deviceName string) error {
	if _, err := run("losetup", "-d", deviceName); err != nil {
		return errors.Annotatef(err, "detaching loop device %q", deviceName)
	}
	return nil
}
