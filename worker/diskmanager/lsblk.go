// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build linux

package diskmanager

import (
	"bufio"
	"bytes"
	"fmt"
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
	// values for the TYPE column that we care about

	typeDisk = "disk"
	typeLoop = "loop"
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

	logger.Tracef("executing lsblk")
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

		// We may later want to expand this, e.g. to handle lvm,
		// dmraid, crypt, etc., but this is enough to cover bases
		// for now.
		switch deviceType {
		case typeDisk, typeLoop:
		default:
			logger.Tracef("ignoring %q type device: %+v", deviceType, dev)
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

		// Add additional information from sysfs.
		if err := addHardwareInfo(&dev); err != nil {
			logger.Errorf(
				"error getting hardware info for %q from sysfs: %v",
				dev.DeviceName, err,
			)
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

// addHardwareInfo adds additional information about the hardware, and how it is
// attached to the machine, to the given BlockDevice.
func addHardwareInfo(dev *storage.BlockDevice) error {
	logger.Tracef(`executing "udevadm info" for %s`, dev.DeviceName)
	output, err := exec.Command(
		"udevadm", "info",
		"-q", "property",
		"--path", "/block/"+dev.DeviceName,
	).Output()
	if err != nil {
		msg := "udevadm failed"
		if output := bytes.TrimSpace(output); len(output) > 0 {
			msg += fmt.Sprintf(" (%s)", output)
		}
		return errors.Annotate(err, msg)
	}

	var devpath, idBus, idSerial string

	s := bufio.NewScanner(bytes.NewReader(output))
	for s.Scan() {
		line := s.Text()
		sep := strings.IndexRune(line, '=')
		if sep == -1 {
			logger.Debugf("unexpected udevadm output line: %q", line)
			continue
		}
		key, value := line[:sep], line[sep+1:]
		switch key {
		case "DEVPATH":
			devpath = value
		case "ID_BUS":
			idBus = value
		case "ID_SERIAL":
			idSerial = value
		default:
			logger.Tracef("ignoring line: %q", line)
		}
	}
	if err := s.Err(); err != nil {
		return errors.Annotate(err, "cannot parse udevadm output")
	}

	if idBus != "" && idSerial != "" {
		// ID_BUS will be something like "scsi" or "ata";
		// ID_SERIAL will be soemthing like ${MODEL}_${SERIALNO};
		// and together they make up the symlink in /dev/disk/by-id.
		dev.HardwareId = idBus + "-" + idSerial
	}

	// For devices on the SCSI bus, we include the address. This is to
	// support storage providers where the SCSI address may be specified,
	// but the device name can not (and may change, depending on timing).
	if idBus == "scsi" && devpath != "" {
		// DEVPATH will be "<uninteresting stuff>/<SCSI address>/block/<device>".
		re := regexp.MustCompile(fmt.Sprintf(
			`^.*/(\d+):(\d+):(\d+):(\d+)/block/%s$`,
			regexp.QuoteMeta(dev.DeviceName),
		))
		submatch := re.FindStringSubmatch(devpath)
		if submatch != nil {
			// We use the address scheme used by lshw: bus@address. We don't use
			// lshw because it does things we don't need, and that slows it down.
			//
			// In DEVPATH, the address format is "H:C:T:L" ([H]ost, [C]hannel,
			// [T]arget, [L]un); the lshw address format is "H:C.T.L"
			dev.BusAddress = fmt.Sprintf(
				"scsi@%s:%s.%s.%s",
				submatch[1], submatch[2], submatch[3], submatch[4],
			)
		} else {
			logger.Debugf(
				"non matching DEVPATH for %q: %q",
				dev.DeviceName, devpath,
			)
		}
	}

	return nil
}
