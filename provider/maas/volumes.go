// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"strconv"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/storage"
)

const (
	// RootDiskTag is the tag recognised by MAAS as being for
	// the root disk.
	RootDiskTag = "root"

	// TagsAttribute is the name of the pool attribute used
	// to specify tag values for requested volumes.
	TagsAttribute = "tags"
)

type volumeInfo struct {
	name      string
	sizeInGiB uint64
	tags      []string
}

// buildMAASVolumeParameters creates the MAAS volume information to include
// in a request to acquire a MAAS node, based on the supplied storage parameters.
func buildMAASVolumeParameters(args []storage.VolumeParams) ([]volumeInfo, error) {
	if len(args) == 0 {
		return nil, nil
	}
	volumes := make([]volumeInfo, len(args))
	// TODO(wallyworld) - allow root volume to be specified in volume args.
	var rootVolume *volumeInfo
	for i, v := range args {
		info := volumeInfo{
			name: v.Tag.String(),
			// MAAS expects GB, Juju works in GiB.
			sizeInGiB: common.MiBToGiB(uint64(v.Size)) * (humanize.GiByte / humanize.GByte),
		}
		var tags string
		if len(v.Attributes) > 0 {
			tags = v.Attributes[TagsAttribute].(string)
		}
		if len(tags) > 0 {
			// We don't want any spaces in the tags;
			// strip out any just in case.
			tags = strings.Replace(tags, " ", "", 0)
			info.tags = strings.Split(tags, ",")
		}
		volumes[i] = info
	}
	if rootVolume == nil {
		rootVolume = &volumeInfo{sizeInGiB: 0}
	}
	// For now, the root disk size can't be specified.
	if rootVolume.sizeInGiB > 0 {
		return nil, errors.New("root volume size cannot be specified")
	}
	// Root disk always goes first.
	volumesResult := []volumeInfo{*rootVolume}
	volumesResult = append(volumesResult, volumes...)
	return volumesResult, nil
}

// volumes creates the storage volumes corresponding to the
// volume info associated with a MAAS node.
func (mi *maasInstance) volumes() ([]storage.Volume, error) {
	var result []storage.Volume

	deviceInfo, ok := mi.getMaasObject().GetMap()["physicalblockdevice_set"]
	// Older MAAS servers don't support storage.
	if !ok || deviceInfo.IsNil() {
		return result, nil
	}

	tagsMap, ok := mi.getMaasObject().GetMap()["constraint_map"]
	if !ok || tagsMap.IsNil() {
		return nil, errors.NotFoundf("constraint map field")
	}

	devices, err := deviceInfo.GetArray()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, d := range devices {
		deviceAttrs, err := d.GetMap()
		if err != nil {
			return nil, errors.Trace(err)
		}
		// id in devices list is numeric
		id, err := deviceAttrs["id"].GetFloat64()
		if err != nil {
			return nil, errors.Annotate(err, "invalid device id")
		}
		// id in constraint_map field is a string
		idKey := strconv.Itoa(int(id))
		// deviceTag is the volume tag passed
		// into the acquire node call as part
		// of the storage constraints parameter.
		deviceTags, err := tagsMap.GetMap()
		if err != nil {
			return nil, errors.Annotate(err, "invalid constraint map value")
		}

		// Device Tag.
		deviceTagValue, ok := deviceTags[idKey]
		if !ok {
			return nil, errors.Errorf("missing volume tag for id %q", idKey)
		}
		deviceTag, err := deviceTagValue.GetString()
		if err != nil {
			return nil, errors.Annotate(err, "invalid device tag")
		}
		// We don't explicitly allow the root volume to be specified yet.
		if deviceTag == RootDiskTag {
			continue
		}

		// Volume Tag.
		volumeTag, err := names.ParseVolumeTag(deviceTag)
		if err != nil {
			return nil, errors.Trace(err)
		}

		// HardwareId.
		hardwareId, err := deviceAttrs["serial"].GetString()
		if err != nil {
			return nil, errors.Annotate(err, "invalid device serial")
		}

		// VolumeId.
		// First try for id_path.
		deviceId, err := deviceAttrs["id_path"].GetString()
		if err != nil {
			// On VMAAS, id_path not available so try for path instead.
			deviceId, err = deviceAttrs["path"].GetString()
			if err != nil {
				return nil, errors.Annotate(err, "invalid device path")
			}
		}

		// Size.
		sizeinGB, err := deviceAttrs["size"].GetFloat64()
		if err != nil {
			return nil, errors.Annotate(err, "invalid device size")
		}

		vol := storage.Volume{
			Tag:        volumeTag,
			VolumeId:   deviceId,
			HardwareId: hardwareId,
			Size:       uint64(sizeinGB / humanize.MiByte),
			Persistent: false,
		}
		result = append(result, vol)
	}
	return result, nil
}
