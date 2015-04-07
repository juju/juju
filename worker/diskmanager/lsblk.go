// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build linux

package diskmanager

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/juju/errors"

	"github.com/juju/juju/storage"
)

var pairsRE = regexp.MustCompile(`([A-Z]+)=(?:"(.*?)")`)

const (
	// partitionType is the value of the TYPE column
	// in lsblk output for partitions.
	partitionType = "part"
)

func init() {
	DefaultListBlockDevices = listBlockDevices
}

func listBlockDevices() ([]storage.BlockDevice, error) {
	columns := []string{
		"KNAME",      // kernel name
		"SIZE",       // size
		"LABEL",      // filesystem label
		"UUID",       // filesystem UUID
		"FSTYPE",     // filesystem type
		"TYPE",       // device type
		"MOUNTPOINT", // moint point
	}

	logger.Debugf("executing lsblk")
	output, err := exec.Command(
		"lsblk",
		"-b", // output size in bytes
		"-P", // output fields as key=value pairs
		"-o", strings.Join(columns, ","),
	).Output()
	if err != nil {
		return nil, errors.Annotate(
			err, "cannot list block devices: lsblk failed",
		)
	}

	blockDeviceMap := make(map[string]storage.BlockDevice)
	s := bufio.NewScanner(bytes.NewReader(output))
	for s.Scan() {
		pairs := pairsRE.FindAllStringSubmatch(s.Text(), -1)
		var dev storage.BlockDevice
		var deviceType string
		for _, pair := range pairs {
			switch pair[1] {
			case "KNAME":
				dev.DeviceName = pair[2]
			case "SIZE":
				size, err := strconv.ParseUint(pair[2], 10, 64)
				if err != nil {
					logger.Errorf(
						"invalid size %q from lsblk: %v", pair[2], err,
					)
				} else {
					dev.Size = size / bytesInMiB
				}
			case "LABEL":
				dev.Label = pair[2]
			case "UUID":
				dev.UUID = pair[2]
			case "FSTYPE":
				dev.FilesystemType = pair[2]
			case "TYPE":
				deviceType = pair[2]
			case "MOUNTPOINT":
				dev.MountPoint = pair[2]
			default:
				logger.Debugf("unexpected field from lsblk: %q", pair[1])
			}
		}

		// Partitions may not be used, as there is no guarantee that the
		// partition will remain available (and we don't model hierarchy).
		if deviceType == partitionType {
			logger.Debugf("ignoring partition: %+v", dev)
			continue
		}

		// Check if the block device is in use. We need to know this so we can
		// issue an error if the user attempts to allocate an in-use disk to a
		// unit.
		dev.InUse, err = blockDeviceInUse(dev)
		if os.IsNotExist(err) {
			// In LXC containers, lsblk will show the block devices of the
			// host, but the devices will typically not be present.
			continue
		} else if err != nil {
			logger.Errorf(
				"error checking if %q is in use: %v", dev.DeviceName, err,
			)
			// We cannot detect, so err on the side of caution and default to
			// "in use" so the device cannot be used.
			dev.InUse = true
		}
		blockDeviceMap[dev.DeviceName] = dev
	}
	if err := s.Err(); err != nil {
		return nil, errors.Annotate(err, "cannot parse lsblk output")
	}

	blockDevices := make([]storage.BlockDevice, 0, len(blockDeviceMap))
	for _, dev := range blockDeviceMap {
		blockDevices = append(blockDevices, dev)
	}
	return blockDevices, nil
}

// blockDeviceInUse checks if the specified block device
// is in use by attempting to open the device exclusively.
//
// If the error returned satisfies os.IsNotExists, then
// the device will be ignored altogether.
var blockDeviceInUse = func(dev storage.BlockDevice) (bool, error) {
	f, err := os.OpenFile("/dev/"+dev.DeviceName, os.O_EXCL, 0)
	if err == nil {
		f.Close()
		return false, nil
	}
	perr, ok := err.(*os.PathError)
	if !ok {
		return false, err
	}
	// open(2): "In general, the behavior of O_EXCL is undefined if
	// it is used without O_CREAT. There is one exception: on Linux
	// 2.6 and later, O_EXCL can be used without O_CREAT if pathname
	// refers to a block device. If the block device is in use by the
	// system  (e.g., mounted), open() fails with the error EBUSY."
	if errno, _ := perr.Err.(syscall.Errno); errno == syscall.EBUSY {
		return true, nil
	}
	return false, err
}
