// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/dustin/go-humanize"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gomaasapi"
	"github.com/juju/schema"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/storage"
)

const (
	// maasStorageProviderType is the name of the storage provider
	// used to specify storage when acquiring MAAS nodes.
	maasStorageProviderType = storage.ProviderType("maas")

	// rootDiskLabel is the label recognised by MAAS as being for
	// the root disk.
	rootDiskLabel = "root"

	// tagsAttribute is the name of the pool attribute used
	// to specify tag values for requested volumes.
	tagsAttribute = "tags"
)

// StorageProviderTypes implements storage.ProviderRegistry.
func (*maasEnviron) StorageProviderTypes() ([]storage.ProviderType, error) {
	return []storage.ProviderType{maasStorageProviderType}, nil
}

// StorageProvider implements storage.ProviderRegistry.
func (*maasEnviron) StorageProvider(t storage.ProviderType) (storage.Provider, error) {
	if t == maasStorageProviderType {
		return maasStorageProvider{}, nil
	}
	return nil, errors.NotFoundf("storage provider %q", t)
}

// maasStorageProvider allows volumes to be specified when a node is acquired.
type maasStorageProvider struct{}

var storageConfigFields = schema.Fields{
	tagsAttribute: schema.OneOf(
		schema.List(schema.String()),
		schema.String(),
	),
}

var storageConfigChecker = schema.FieldMap(
	storageConfigFields,
	schema.Defaults{
		tagsAttribute: schema.Omit,
	},
)

type storageConfig struct {
	tags []string
}

func newStorageConfig(attrs map[string]interface{}) (*storageConfig, error) {
	out, err := storageConfigChecker.Coerce(attrs, nil)
	if err != nil {
		return nil, errors.Annotate(err, "validating MAAS storage config")
	}
	coerced := out.(map[string]interface{})
	var tags []string
	switch v := coerced[tagsAttribute].(type) {
	case []string:
		tags = v
	case string:
		fields := strings.Split(v, ",")
		for _, f := range fields {
			f = strings.TrimSpace(f)
			if len(f) == 0 {
				continue
			}
			if i := strings.IndexFunc(f, unicode.IsSpace); i >= 0 {
				return nil, errors.Errorf("tags may not contain whitespace: %q", f)
			}
			tags = append(tags, f)
		}
	}
	return &storageConfig{tags: tags}, nil
}

// ValidateConfig is defined on the Provider interface.
func (maasStorageProvider) ValidateConfig(cfg *storage.Config) error {
	_, err := newStorageConfig(cfg.Attrs())
	return errors.Trace(err)
}

// Supports is defined on the Provider interface.
func (maasStorageProvider) Supports(k storage.StorageKind) bool {
	return k == storage.StorageKindBlock
}

// Scope is defined on the Provider interface.
func (maasStorageProvider) Scope() storage.Scope {
	return storage.ScopeEnviron
}

// Dynamic is defined on the Provider interface.
func (maasStorageProvider) Dynamic() bool {
	return false
}

// Releasable is defined on the Provider interface.
func (maasStorageProvider) Releasable() bool {
	return false
}

// DefaultPools is defined on the Provider interface.
func (maasStorageProvider) DefaultPools() []*storage.Config {
	return nil
}

// VolumeSource is defined on the Provider interface.
func (maasStorageProvider) VolumeSource(providerConfig *storage.Config) (storage.VolumeSource, error) {
	// Dynamic volumes not supported.
	return nil, errors.NotSupportedf("volumes")
}

// FilesystemSource is defined on the Provider interface.
func (maasStorageProvider) FilesystemSource(providerConfig *storage.Config) (storage.FilesystemSource, error) {
	return nil, errors.NotSupportedf("filesystems")
}

type volumeInfo struct {
	name     string
	sizeInGB uint64
	tags     []string
}

// mibToGB converts the value in MiB to GB.
// Juju works in MiB, MAAS expects GB.
func mibToGb(m uint64) uint64 {
	return common.MiBToGiB(m) * (humanize.GiByte / humanize.GByte)
}

// buildMAASVolumeParameters creates the MAAS volume information to include
// in a request to acquire a MAAS node, based on the supplied storage parameters.
func buildMAASVolumeParameters(args []storage.VolumeParams, cons constraints.Value) ([]volumeInfo, error) {
	if len(args) == 0 && cons.RootDisk == nil {
		return nil, nil
	}
	volumes := make([]volumeInfo, len(args)+1)
	rootVolume := volumeInfo{name: rootDiskLabel}
	if cons.RootDisk != nil {
		rootVolume.sizeInGB = mibToGb(*cons.RootDisk)
	}
	volumes[0] = rootVolume
	for i, v := range args {
		cfg, err := newStorageConfig(v.Attributes)
		if err != nil {
			return nil, errors.Trace(err)
		}
		info := volumeInfo{
			name:     v.Tag.Id(),
			sizeInGB: mibToGb(v.Size),
			tags:     cfg.tags,
		}
		volumes[i+1] = info
	}
	return volumes, nil
}

// volumes creates the storage volumes and attachments
// corresponding to the volume info associated with a MAAS node.
func (mi *maas1Instance) volumes(
	mTag names.MachineTag, requestedVolumes []names.VolumeTag,
) (
	[]storage.Volume, []storage.VolumeAttachment, error,
) {
	var volumes []storage.Volume
	var attachments []storage.VolumeAttachment

	deviceInfo, ok := mi.maasObject.GetMap()["physicalblockdevice_set"]
	// Older MAAS servers don't support storage.
	if !ok || deviceInfo.IsNil() {
		return volumes, attachments, nil
	}

	labelsMap, ok := mi.maasObject.GetMap()["constraint_map"]
	if !ok || labelsMap.IsNil() {
		return nil, nil, errors.NotFoundf("constraint map field")
	}

	devices, err := deviceInfo.GetArray()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	// deviceLabel is the volume label passed
	// into the acquire node call as part
	// of the storage constraints parameter.
	deviceLabels, err := labelsMap.GetMap()
	if err != nil {
		return nil, nil, errors.Annotate(err, "invalid constraint map value")
	}

	// Set up a collection of volumes tags which
	// we specifically asked for when the node was acquired.
	validVolumes := set.NewStrings()
	for _, v := range requestedVolumes {
		validVolumes.Add(v.Id())
	}

	for _, d := range devices {
		deviceAttrs, err := d.GetMap()
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		// id in devices list is numeric
		id, err := deviceAttrs["id"].GetFloat64()
		if err != nil {
			return nil, nil, errors.Annotate(err, "invalid device id")
		}
		// id in constraint_map field is a string
		idKey := strconv.Itoa(int(id))

		// Device Label.
		deviceLabelValue, ok := deviceLabels[idKey]
		if !ok {
			logger.Debugf("acquire maas node: missing volume label for id %q", idKey)
			continue
		}
		deviceLabel, err := deviceLabelValue.GetString()
		if err != nil {
			return nil, nil, errors.Annotate(err, "invalid device label")
		}
		// We don't explicitly allow the root volume to be specified yet.
		if deviceLabel == rootDiskLabel {
			continue
		}
		// We only care about the volumes we specifically asked for.
		if !validVolumes.Contains(deviceLabel) {
			continue
		}

		// HardwareId and DeviceName.
		// First try for id_path.
		idPathPrefix := "/dev/disk/by-id/"
		hardwareId, err := deviceAttrs["id_path"].GetString()
		var deviceName string
		if err == nil {
			if !strings.HasPrefix(hardwareId, idPathPrefix) {
				return nil, nil, errors.Errorf("invalid device id %q", hardwareId)
			}
			hardwareId = hardwareId[len(idPathPrefix):]
		} else {
			// On VMAAS, id_path not available so try for path instead.
			deviceName, err = deviceAttrs["name"].GetString()
			if err != nil {
				return nil, nil, errors.Annotate(err, "invalid device name")
			}
		}

		// Size.
		sizeinBytes, err := deviceAttrs["size"].GetFloat64()
		if err != nil {
			return nil, nil, errors.Annotate(err, "invalid device size")
		}

		volumeTag := names.NewVolumeTag(deviceLabel)
		vol := storage.Volume{
			volumeTag,
			storage.VolumeInfo{
				VolumeId:   volumeTag.String(),
				HardwareId: hardwareId,
				Size:       uint64(sizeinBytes / humanize.MiByte),
				Persistent: false,
			},
		}
		volumes = append(volumes, vol)

		attachment := storage.VolumeAttachment{
			volumeTag,
			mTag,
			storage.VolumeAttachmentInfo{
				DeviceName: deviceName,
				ReadOnly:   false,
			},
		}
		attachments = append(attachments, attachment)
	}
	return volumes, attachments, nil
}

func (mi *maas2Instance) volumes(
	mTag names.MachineTag, requestedVolumes []names.VolumeTag,
) (
	[]storage.Volume, []storage.VolumeAttachment, error,
) {
	if mi.constraintMatches.Storage == nil {
		return nil, nil, errors.NotFoundf("constraint storage mapping")
	}

	var volumes []storage.Volume
	var attachments []storage.VolumeAttachment

	// Set up a collection of volumes tags which
	// we specifically asked for when the node was acquired.
	validVolumes := set.NewStrings()
	for _, v := range requestedVolumes {
		validVolumes.Add(v.Id())
	}

	for label, devices := range mi.constraintMatches.Storage {
		// We don't explicitly allow the root volume to be specified yet.
		if label == rootDiskLabel {
			continue
		}

		// We only care about the volumes we specifically asked for.
		if !validVolumes.Contains(label) {
			continue
		}

		// There should be exactly one block device per label.
		if len(devices) == 0 {
			continue
		} else if len(devices) > 1 {
			// This should never happen, as we only request one block
			// device per label. If it does happen, we'll just report
			// the first block device and log this warning.
			logger.Warningf(
				"expected 1 block device for label %s, received %d",
				label, len(devices),
			)
		}

		device := devices[0]
		volumeTag := names.NewVolumeTag(label)
		vol := storage.Volume{
			volumeTag,
			storage.VolumeInfo{
				VolumeId:   volumeTag.String(),
				Size:       device.Size() / humanize.MiByte,
				Persistent: false,
			},
		}
		attachment := storage.VolumeAttachment{
			volumeTag,
			mTag,
			storage.VolumeAttachmentInfo{
				ReadOnly: false,
			},
		}

		const devDiskByIdPrefix = "/dev/disk/by-id/"
		const devPrefix = "/dev/"

		if blockDev, ok := device.(gomaasapi.BlockDevice); ok {
			// Handle a block device specifically that way the path used
			// by Juju will always be a persistent path.
			idPath := blockDev.IDPath()
			if idPath == devPrefix+blockDev.Name() {
				// On vMAAS (i.e. with virtio), the device name
				// will be stable, and is what is used to form
				// id_path.
				deviceName := idPath[len(devPrefix):]
				attachment.DeviceName = deviceName
			} else if strings.HasPrefix(idPath, devDiskByIdPrefix) {
				const wwnPrefix = "wwn-"
				id := idPath[len(devDiskByIdPrefix):]
				if strings.HasPrefix(id, wwnPrefix) {
					vol.WWN = id[len(wwnPrefix):]
				} else {
					vol.HardwareId = id
				}
			} else {
				// It's neither /dev/<name> nor /dev/disk/by-id/<hardware-id>,
				// so set it as the device link and hope for
				// the best. At worst, the path won't exist
				// and the storage will remain pending.
				attachment.DeviceLink = idPath
			}
		} else {
			// Handle all other storage devices using the path MAAS provided.
			// In the case of partitions the path is always stable because its
			// based on the GUID of the partition using the dname path.
			attachment.DeviceLink = device.Path()
		}

		volumes = append(volumes, vol)
		attachments = append(attachments, attachment)
	}
	return volumes, attachments, nil
}
