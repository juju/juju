// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskmanager

import (
	"bufio"
	"bytes"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/storage"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.diskmanager")

const (
	// listBlockDevicesPeriod is the time period between block device listings.
	// Unfortunately Linux's inotify does not work with virtual filesystems, so
	// polling it is.
	listBlockDevicesPeriod = time.Second * 30

	// bytesInMiB is the number of bytes in a MiB.
	bytesInMiB = 1024 * 1024
)

// BlockDeviceSetter is an interface that is supplied to
// NewWorker for setting block devices for the local host.
type BlockDeviceSetter interface {
	SetMachineBlockDevices([]storage.BlockDevice) error
}

// NewWorker returns a worker that lists block devices
// attached to the machine, and records them in state.
func NewWorker(b BlockDeviceSetter) (worker.Worker, error) {
	switch version.Current.OS {
	default:
		logger.Infof(
			"block device support has not been implemented for %s",
			version.Current.OS,
		)
		// Eventually we should support listing disks attached to
		// a Windows machine. For now, return a no-op worker.
		return worker.NewNoOpWorker(), nil
	case version.Ubuntu:
		return worker.NewPeriodicWorker(func(stop <-chan struct{}) error {
			blockDevices, err := listBlockDevices()
			if err != nil {
				return err
			}
			return b.SetMachineBlockDevices(blockDevices)
		}, listBlockDevicesPeriod), nil
	}
}

var pairsRE = regexp.MustCompile(`([A-Z]+)=(?:"(.*?)")`)

func listBlockDevices() ([]storage.BlockDevice, error) {
	columns := []string{
		"KNAME", // kernel name
		"SIZE",  // size
		"LABEL", // filesystem label
		"UUID",  // filesystem UUID
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
			default:
				logger.Debugf("unexpected field from lsblk: %q", pair[1])
			}
		}

		// Check if the block device is in use. We need to know this so we can
		// issue an error if the user attempts to allocate an in-use disk to a
		// unit.
		dev.InUse, err = blockDeviceInUse(dev)
		if err != nil {
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
