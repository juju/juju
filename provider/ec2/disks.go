// Copyright 2011-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"github.com/juju/errors"
	"launchpad.net/goamz/ec2"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/storage"
)

const (
	ebsStorageSource = "ebs"

	// minRootDiskSize is the minimum/default size (in mebibytes) for ec2 root disks.
	minRootDiskSize uint64 = 8 * 1024

	// volumeSizeMax is the maximum disk size (in mebibytes) for EBS volumes.
	volumeSizeMax = 1024 * 1024 // 1024 GiB

	// iopsMin is the minimum IOPS value.
	iopsMin = 100

	// iopsMax is the maximum IOPS value.
	iopsMax = 4000

	// iopsSizeRatioMax is the maximum ratio of IOPS to size (in GiB).
	iopsSizeRatioMax = 30
)

// getBlockDeviceMappings translates a StartInstanceParams into
// BlockDeviceMappings.
//
// The first entry is always the root disk mapping, instance stores
// (ephemeral disks) last.
func getBlockDeviceMappings(
	virtType string,
	args *environs.StartInstanceParams,
) (
	[]ec2.BlockDeviceMapping, map[storage.Constraints][]storage.BlockDevice, error,
) {
	rootDiskSize := minRootDiskSize
	if args.Constraints.RootDisk != nil {
		if *args.Constraints.RootDisk >= minRootDiskSize {
			rootDiskSize = *args.Constraints.RootDisk
		} else {
			logger.Infof(
				"Ignoring root-disk constraint of %dM because it is smaller than the EC2 image size of %dM",
				*args.Constraints.RootDisk,
				minRootDiskSize,
			)
		}
	}

	// The first block device is for the root disk.
	blockDeviceMappings := []ec2.BlockDeviceMapping{{
		DeviceName: "/dev/sda1",
		VolumeSize: int64(mibToGib(rootDiskSize)),
	}}

	// Not all machines have this many instance stores.
	// Instances will be started with as many of the
	// instance stores as they can support.
	blockDeviceMappings = append(blockDeviceMappings, []ec2.BlockDeviceMapping{{
		VirtualName: "ephemeral0",
		DeviceName:  "/dev/sdb",
	}, {
		VirtualName: "ephemeral1",
		DeviceName:  "/dev/sdc",
	}, {
		VirtualName: "ephemeral2",
		DeviceName:  "/dev/sdd",
	}, {
		VirtualName: "ephemeral3",
		DeviceName:  "/dev/sde",
	}}...)

	// TODO(axw) if preference is to use ephemeral, use ephemeral
	// until the instance stores run out. We'll need to know how
	// many there are and how big each one is. We also need to
	// unmap ephemeral0 in cloud-init.
	disks := make([][]storage.BlockDevice, len(args.Disks))

	mappings := make([]ec2.BlockDeviceMapping, len(args.Disks))
	for i, diskCons := range args.Disks {
		// Check minimum constraints can be satisfied.
		if err := validateConstraints(&diskCons); err != nil {
			// TODO(axw) we need to determine if another storage
			// provider can provide the disks, when there are
			// other storage providers.
			return nil, nil, errors.Annotate(err, "cannot satisfy disk constraints")
		}
		mapping := ec2.BlockDeviceMapping{
			VolumeSize: int64(mibToGib(diskCons.Preferred.Size)),
			IOPS:       int64(diskCons.Preferred.IOPS),
			// TODO(axw) when we model storage persistence,
			// honour the constraint.
			//DeleteOnTermination: diskCons.Preferred.Persistent,
		}
		if mapping.IOPS > 0 {
			mapping.VolumeType = "io1"
			if mapping.IOPS < iopsMin {
				mapping.IOPS = iopsMin
			} else if mapping.IOPS > iopsMax {
				mapping.IOPS = iopsMax
			}
		} else {
			// TODO(axw) we need a way of specifying the volume type,
			// so users can request an SSD.
			mapping.VolumeType = "standard"
		}
		mappings[i] = mapping
	}

	nextDeviceName := blockDeviceNamer(virtType == paravirtual)
	createBlockDevice := func(mapping ec2.BlockDeviceMapping) (name string, _ storage.BlockDevice, _ error) {
		requestDeviceName, actualDeviceName, err := nextDeviceName()
		if err != nil {
			// Can't allocate any more disks.
			//
			// TODO(axw) we need to determine if another storage
			// provider can provide the disks, when there are
			// other storage providers.
			return "", storage.BlockDevice{}, errors.Annotate(err, "cannot satisfy disk constraints")
		}
		disk := storage.BlockDevice{
			DeviceName: actualDeviceName,
			Size:       gibToMib(uint64(mapping.VolumeSize)),
			// TODO(axw) record other properties (media type, IOPS, ...)
		}
		return requestDeviceName, disk, nil
	}

	// First go through and create the minimum count of the preferred
	// constraints. Once we've done that, we'll go through each in turn
	// creating up to the preference until the number of available disks
	// runs out.
	for i, mapping := range mappings {
		if mapping.VolumeSize == 0 {
			continue
		}
		for j := uint64(0); j < args.Disks[i].Minimum.Count; j++ {
			deviceName, disk, err := createBlockDevice(mapping)
			if err != nil {
				return nil, nil, err
			}
			mapping.DeviceName = deviceName
			disks[i] = append(disks[i], disk)
			blockDeviceMappings = append(blockDeviceMappings, mapping)
		}
	}
	for i, mapping := range mappings {
		if mapping.VolumeSize == 0 {
			continue
		}
		for j := args.Disks[i].Minimum.Count; j < args.Disks[i].Preferred.Count; j++ {
			deviceName, disk, err := createBlockDevice(mapping)
			if err != nil {
				// Cannot create any more disks. Not an actual error, as this
				// is just a preference.
				logger.Infof("cannot create any more disks: %v", err)
				break
			}
			mapping.DeviceName = deviceName
			disks[i] = append(disks[i], disk)
			blockDeviceMappings = append(blockDeviceMappings, mapping)
		}
	}

	var diskConstraintMapping map[storage.Constraints][]storage.BlockDevice
	for i, disks := range disks {
		if len(disks) > 0 {
			if diskConstraintMapping == nil {
				diskConstraintMapping = make(map[storage.Constraints][]storage.BlockDevice)
			}
			diskConstraintMapping[args.Disks[i]] = disks
		}
	}
	return blockDeviceMappings, diskConstraintMapping, nil
}

// validateConstraints validates the specified storage constraints.
func validateConstraints(cons *storage.Constraints) error {
	if cons.Source != "" && cons.Source != ebsStorageSource {
		return errors.Errorf("designated for storage source %q", cons.Source)
	}
	if cons.Minimum.IOPS > iopsMax {
		return errors.Errorf("%d IOPS exceeds the maximum of %d", cons.Minimum.IOPS, iopsMax)
	}
	if cons.Minimum.Size > volumeSizeMax {
		return errors.Errorf("%d MiB exceeds the maximum of %d MiB", cons.Minimum.Size, volumeSizeMax)
	}
	if cons.Minimum.Persistent {
		// TODO(axw) when we model storage persistence, handle persistent.
		return errors.NotSupportedf("persistent volumes")
	}
	return nil
}

// mibToGib converts mebibytes to gibibytes.
// AWS expects GiB, we work in MiB; round up
// to nearest GiB.
func mibToGib(m uint64) uint64 {
	return (m + 1023) / 1024
}

// gibToMib converts gibibytes to mebibytes.
func gibToMib(g uint64) uint64 {
	return g / 1024
}

var errTooManyVolumes = errors.New("too many EBS volumes to attach")

// blockDeviceNamer returns a function that cycles through block device names.
//
// The returned function returns the device name that should be used in
// requests to the EC2 API, and and also the (kernel) device name as it
// will appear on the machine.
//
// See http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/block-device-mapping-concepts.html
func blockDeviceNamer(numbers bool) func() (requestName, actualName string, err error) {
	const (
		// deviceLetterMin is the first letter to use for EBS block device names.
		deviceLetterMin = 'f'
		// deviceLetterMax is the last letter to use for EBS block device names.
		deviceLetterMax = 'p'
		// deviceNumMax is the maximum value for trailing numbers on block device name.
		deviceNumMax = 6
		// devicePrefix is the prefix for device names specified when creating volumes.
		devicePrefix = "/dev/sd"
		// renamedDevicePrefix is the prefix for device names after they have
		// been renamed. This should replace "devicePrefix" in the device name
		// when recording the block device info in state.
		renamedDevicePrefix = "xvd"
	)
	var n int
	return func() (string, string, error) {
		letter := deviceLetterMin + (n / deviceNumMax)
		if letter > deviceLetterMax {
			return "", "", errTooManyVolumes
		}
		deviceName := devicePrefix + string(letter)
		if numbers {
			deviceName += string('1' + (n % deviceNumMax))
		}
		n++
		realDeviceName := renamedDevicePrefix + deviceName[len(devicePrefix):]
		return deviceName, realDeviceName, nil
	}
}
