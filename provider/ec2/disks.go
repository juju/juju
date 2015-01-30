// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"strconv"

	"github.com/juju/errors"
	"gopkg.in/amz.v2/ec2"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/storage"
)

const (
	// minRootDiskSizeMiB is the minimum/default size (in mebibytes) for ec2 root disks.
	minRootDiskSizeMiB uint64 = 8 * 1024

	// provisionedIopsvolumeSizeMinGiB is the minimum disk size (in gibibytes)
	// for provisioned IOPS EBS volumes.
	provisionedIopsvolumeSizeMinGiB = 10 // 10 GiB

	// volumeSizeMaxGiB is the maximum disk size (in gibibytes) for EBS volumes.
	volumeSizeMaxGiB = 1024 // 1024 GiB

	// maxProvisionedIopsSizeRatio is the maximum allowed ratio of IOPS to
	// size (in GiB), for provisioend IOPS volumes.
	maxProvisionedIopsSizeRatio = 30
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
	[]ec2.BlockDeviceMapping, []storage.Volume, []storage.VolumeAttachment, error,
) {
	rootDiskSizeMiB := minRootDiskSizeMiB
	if args.Constraints.RootDisk != nil {
		if *args.Constraints.RootDisk >= minRootDiskSizeMiB {
			rootDiskSizeMiB = *args.Constraints.RootDisk
		} else {
			logger.Infof(
				"Ignoring root-disk constraint of %dM because it is smaller than the EC2 image size of %dM",
				*args.Constraints.RootDisk,
				minRootDiskSizeMiB,
			)
		}
	}

	// The first block device is for the root disk.
	blockDeviceMappings := []ec2.BlockDeviceMapping{{
		DeviceName: "/dev/sda1",
		VolumeSize: int64(mibToGib(rootDiskSizeMiB)),
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

	volumes := make([]storage.Volume, len(args.Volumes))
	attachments := make([]storage.VolumeAttachment, len(args.Volumes))
	nextDeviceName := blockDeviceNamer(virtType == paravirtual)
	for i, params := range args.Volumes {
		if params.Provider != EBS_ProviderType {
			continue
		}
		if params.Attachment == nil {
			return nil, nil, nil, errors.NotImplementedf("allocating unattached volumes")
		}
		mapping := ec2.BlockDeviceMapping{
			VolumeSize: int64(mibToGib(params.Size)),
			// TODO(axw) DeleteOnTermination
		}
		// Translate user values for storage provider parameters.
		// TODO(wallyworld) - remove type assertions when juju/schema is used
		options := TranslateUserEBSOptions(params.Attributes)
		if v, ok := options[EBS_VolumeType]; ok && v != "" {
			mapping.VolumeType = v.(string)
		}
		if v, ok := options[EBS_IOPS]; ok && v != "" {
			var err error
			mapping.IOPS, err = strconv.ParseInt(v.(string), 10, 64)
			if err != nil {
				return nil, nil, nil, errors.Annotatef(err, "invalid iops value %v, expected integer", v)
			}
		}

		// Check mapping is valid (minus the device name).
		if err := validateBlockDeviceMapping(mapping); err != nil {
			return nil, nil, nil, errors.Annotate(err, "invalid volume parameters")
		}
		requestDeviceName, actualDeviceName, err := nextDeviceName()
		if err != nil {
			// Can't attach any more volumes.
			return nil, nil, nil, err
		}
		mapping.DeviceName = requestDeviceName

		volume := storage.Volume{
			Tag:  params.Tag,
			Size: gibToMib(uint64(mapping.VolumeSize)),
			// VolumeId will be filled in once the instance has
			// been created, which will create the volumes too.
		}
		attachment := storage.VolumeAttachment{
			Volume:     params.Tag,
			Machine:    params.Attachment.Machine,
			DeviceName: actualDeviceName,
		}
		blockDeviceMappings = append(blockDeviceMappings, mapping)
		volumes[i] = volume
		attachments[i] = attachment
	}
	return blockDeviceMappings, volumes, attachments, nil
}

// validateBlockDeviceMapping validates a block device mapping,
// excluding the device name.
func validateBlockDeviceMapping(m ec2.BlockDeviceMapping) error {
	if m.VolumeSize > volumeSizeMaxGiB {
		return errors.Errorf("%d GiB exceeds the maximum of %d GiB", m.VolumeSize, volumeSizeMaxGiB)
	}
	if m.VolumeType == "io1" {
		if m.VolumeSize < provisionedIopsvolumeSizeMinGiB {
			return errors.Errorf(
				"volume size is %d GiB, must be at least %d GiB for provisioned IOPS",
				m.VolumeSize,
				provisionedIopsvolumeSizeMinGiB,
			)
		}
	}
	if m.IOPS > 0 {
		if m.VolumeType != "io1" {
			return errors.Errorf("IOPS specified, but volume type is %q", m.VolumeType)
		}
		minSize := m.IOPS / maxProvisionedIopsSizeRatio
		if m.VolumeSize < minSize {
			return errors.Errorf(
				"volume size is %d GiB, must be at least %d GiB to support %d IOPS",
				m.VolumeSize, minSize, m.IOPS,
			)
		}
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
	return g * 1024
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
	letterRepeats := 1
	if numbers {
		letterRepeats = deviceNumMax
	}
	return func() (string, string, error) {
		letter := deviceLetterMin + (n / letterRepeats)
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
