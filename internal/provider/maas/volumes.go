// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"context"
	"strings"
	"unicode"

	"github.com/dustin/go-humanize"
	"github.com/juju/collections/set"
	"github.com/juju/gomaasapi/v3"
	"github.com/juju/names/v6"
	"github.com/juju/schema"

	"github.com/juju/juju/core/constraints"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/storage"
)

// maasStorageProvider provides a [storage.Provider] implementation that is not
// capable of provisioning any block devices or filesystems within a Juju model
// on behalf of charm storage requirements.
//
// This provider solely exists to support the provisioning of root disk volumes
// for new machines being provisioned within MAAS.
type maasStorageProvider struct{}

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

// getMAASProviderConfigChecker returns a [schema.Checker] capable of checking
// the configuration of a [maasStorageProvider].
func getMAASProviderConfigChecker() schema.Checker {
	return schema.FieldMap(
		schema.Fields{
			tagsAttribute: schema.OneOf(
				schema.List(schema.String()),
				schema.String(),
			),
		},
		schema.Defaults{
			tagsAttribute: schema.Omit,
		},
	)
}

// RecommendedPoolForKind returns the recommended storage pool to use for
// the given storage kind. If no pool can be recommended nil is returned. For
// the MAAS environ only builtin IAAS pools are returned.
//
// The [maasStorageProvider] is never recommended as it can only ever be used
// for provisioning root disks on new machines.
//
// Implements [storage.ProviderRegistry] interface.
func (*maasEnviron) RecommendedPoolForKind(
	kind storage.StorageKind,
) *storage.Config {
	return common.GetCommonRecommendedIAASPoolForKind(kind)
}

// StorageProviderTypes returns the set of provider types supported for
// provisioning storage on behalf of this environ.
//
// Implements [storage.ProviderRegistry] interface.
func (*maasEnviron) StorageProviderTypes() ([]storage.ProviderType, error) {
	return append(
		common.CommonIAASStorageProviderTypes(),
		maasStorageProviderType,
	), nil
}

// StorageProvider returns the implementation of [storage.Provider] for the
// given storage provider type. See [maasEnviron.StorageProviderTypes] for the
// set of supported provider types.
//
// Implements [storage.ProviderRegistry] interface.
//
// The following errors may be returned:
// - [github.com/juju/juju/core/errors.NotFound] when no provider exists for the
// supplied [storage.ProviderType].
func (*maasEnviron) StorageProvider(t storage.ProviderType) (storage.Provider, error) {
	switch t {
	case maasStorageProviderType:
		return maasStorageProvider{}, nil
	default:
		return common.GetCommonIAASStorageProvider(t)
	}
}

// TagsFromAttributes takes the attributes from a storage pool that is using
// the [maasStorageProvider] and returns the set of tags that have been
// configured on the pool if any. The order of tags returned is not guaranteed
// to match the order supplied in attributes.
//
// Tags are extracted from the attributes using the key "tags" and may be either
// a string that is a comma-separated list of tags, a string slice of tags or
// a slice of any only containing strings.
//
// If a tag has either leading or trailing white space it will be stripped from
// the output. Tags may not contain white space within the tag. See expected
// errors below.
//
// The following errors may be returned:
// - [coreerrors.NotValid] if the attributes do not meet the schema requirements
// of [maasStorageProvider].
// - [coreerrors.NotSupported] if any of the tags contains a white space
// character.
func (maasStorageProvider) TagsFromAttributes(attrs map[string]any) (
	[]string, error,
) {
	out, err := getMAASProviderConfigChecker().Coerce(attrs, nil)
	if err != nil {
		return nil, errors.Errorf(
			"validating MAAS storage provider attributes: %w", err,
		).Add(coreerrors.NotValid)
	}

	coerced := out.(map[string]any)
	var rawTags []string
	switch v := coerced[tagsAttribute].(type) {
	case []any:
		rawTags = make([]string, 0, len(v))
		for _, r := range v {
			// We can safely assume that r is a string because the schema
			// defined by [getMAASProviderConfigChecker] checks on a list of
			// strings.
			rawTags = append(rawTags, r.(string))
		}
	case string:
		rawTags = strings.Split(v, ",")
	}

	var tags []string
	processSingleTag := func(tag string) error {
		tag = strings.TrimSpace(tag)
		if len(tag) == 0 {
			return nil
		}
		if strings.ContainsFunc(tag, unicode.IsSpace) {
			return errors.Errorf(
				"tag %q cannot contain whitespace", tag,
			).Add(coreerrors.NotSupported)
		}
		tags = append(tags, tag)
		return nil
	}

	for _, r := range rawTags {
		if err := processSingleTag(r); err != nil {
			return nil, err
		}
	}
	return tags, nil
}

func (maasStorageProvider) ValidateForK8s(map[string]any) error {
	// no validation required
	return nil
}

// ValidateConfig validates the provided storage pool config that is using
// this [maasStorageProvider] and returns an error if the config is not valid.
//
// This provider only supports tag configuration as part of it's attributes.
// See [maasStorageProvider.TagsFromAttributes].
//
// Implements the [storage.Provider] interface.
//
// The following errors may be returned:
// - [coreerrors.NotValid] if the attributes do not meet the schema requirements
// of [maasStorageProvider].
// - [coreerrors.NotSupported] if any of the tags contains a white space
// character.
func (m maasStorageProvider) ValidateConfig(cfg *storage.Config) error {
	_, err := m.TagsFromAttributes(cfg.Attrs())
	return err
}

// Supports returns true or false to the caller indicating of the given
// [storage.StorageKind] is supported by this provider for provisioning.Supports
//
// [maasStorageProvider] always returns false for all [storage.StorageKind]
// values. This is because the provider can only provision root disks for newly
// created machines in MAAS.
//
// This current implementation should be considered a quirk of the storage
// provider interface as it only supports asking broad provisioning questions
// and not specifics about the context the provisioning is occurring. We
// understand that this is safe to do for the moment as the provisioning of
// machines doesn't interrogate the capabilities of the storage provider.
//
// Implements the [storage.Provider] interface.
func (maasStorageProvider) Supports(k storage.StorageKind) bool {
	return false
}

// Scope returns the [storage.Scope] for which provisioning of storage occurs.
// For MAAS storage is always provisioned via the API and so is always
// considered to be [storage.ScopeEnviron] as the API calls originate from
// within environ.
//
// Implements the [storage.Provider] interface.
func (maasStorageProvider) Scope() storage.Scope {
	return storage.ScopeEnviron
}

// Dynamic indicates to the caller if this provider supports the provisioning of
// dynamic storage. The answer to this question for MAAS is always false.
//
// Implements the [storage.Provider] interface.
func (maasStorageProvider) Dynamic() bool {
	return false
}

// Releasable indicates to the caller if this provider supports releasing of
// storage from it's attached entity. The answer to this question for MAAS is
// always false.
func (maasStorageProvider) Releasable() bool {
	return false
}

// DefaultPools returns the default pools available through the maas provider.
// By default a pool by the same name as the provider is offered. The reason
// this provider returns a default pool that cannot be used for provisioning of
// storage beside machine root disks is so that a model user can set tag
// attributes on the pool which are then used on the root disk in MAAS.
//
// It should be noted that while a default pool is offered, it is never
// recommended. See [maasEnviron.RecommendedPoolForKind].
//
// Implements [storage.Provider] interface.
func (maasStorageProvider) DefaultPools() []*storage.Config {
	// The error for constructing a new storage pool config is disgarded as the
	// [storage.Provider] interface does not support a error return. Because
	// the configuration is static in nature we assume that unit testing will
	// suffice.
	defaultPool, _ := storage.NewConfig(
		maasStorageProviderType.String(),
		maasStorageProviderType,
		storage.Attrs{},
	)

	return []*storage.Config{defaultPool}
}

// VolumeSource is responsible for returning a [storage.VolumeSource] capable of
// provisioning volumes in the model for this storage provider. The
// [maasStorageProvider] does not support the provisioning of volumes outside of
// root disk for a new machines. Because of this an error satisfying
// [github.com/juju/juju/core/errors.NotSupported] is always returned.
//
// Implements [storage.Provider] interface.
func (maasStorageProvider) VolumeSource(_ *storage.Config) (
	storage.VolumeSource, error,
) {
	return nil, errors.Errorf(
		"maas storage provider does not support provisioning of volumes",
	).Add(coreerrors.NotSupported)
}

// FilesystemSource is responsible for returning a [storage.FilesystemSource]
// capable of provisioning filesystems in the model for this storage provider.
// The [maasStorageProvider] does not support the provisioning of file systems
// outside of root disk volumes for  new machines. Because of this an error
// satisfying [github.com/juju/juju/core/errors.NotSupported] is always returned.
//
// Implements [storage.Provider] interface.
func (maasStorageProvider) FilesystemSource(_ *storage.Config) (
	storage.FilesystemSource, error,
) {
	return nil, errors.Errorf(
		"maas storage provider does not support provisioning of filesystems",
	).Add(coreerrors.NotSupported)
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
// in a request to acquire a MAAS node, based on the supplied volume parameters.
// Include in the volume information will also be the root disk volume for the
// node being acquired. This func guarantees that the root disk will be the
// first [volumeInfo] in the returned slice. This is done as MAAS expects this
// ordering.
//
// The volume parameters supplied to this func MUST only be for provider type
// [maasStorageProviderType].
//
// The following errors may be expected:
// - [coreerrors.NotSupported] when either a volume supplied is using a
// provider other then [maasStorageProviderType] or the tags supplied with the
// provider attributes contain whitespace.
// - [coreerrors.NotValid] when the supplied provider attributes do not pass
// validation for a volume.
func buildMAASVolumeParameters(
	args []storage.VolumeParams,
	cons constraints.Value,
) ([]volumeInfo, error) {
	if len(args) == 0 && cons.RootDisk == nil {
		return nil, nil
	}

	// Add one more element to cover the root disk
	volumes := make([]volumeInfo, 0, len(args)+1)

	rootVolume := volumeInfo{name: rootDiskLabel}
	if cons.RootDisk != nil {
		rootVolume.sizeInGB = mibToGb(*cons.RootDisk)
	}
	volumes = append(volumes, rootVolume)

	provider := maasStorageProvider{}
	for _, v := range args {
		if v.Provider != maasStorageProviderType {
			return nil, errors.Errorf(
				"building MAAS volume parameters for provider %q not supported",
				v.Provider,
			).Add(coreerrors.NotSupported)
		}

		tags, err := provider.TagsFromAttributes(v.Attributes)
		if err != nil {
			return nil, errors.Errorf(
				"generating volume %q tags: %w", v.Tag.Id(), err,
			)
		}
		info := volumeInfo{
			name:     v.Tag.Id(),
			sizeInGB: mibToGb(v.Size),
			tags:     tags,
		}
		volumes = append(volumes, info)
	}

	return volumes, nil
}

func (mi *maasInstance) volumes(
	ctx context.Context,
	mTag names.MachineTag, requestedVolumes []names.VolumeTag,
) (
	[]storage.Volume, []storage.VolumeAttachment, error,
) {
	if mi.constraintMatches.Storage == nil {
		return nil, nil, errors.New(
			"constraint storage mapping not found",
		).Add(coreerrors.NotFound)
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
			logger.Warningf(ctx,
				"expected 1 block device for label %s, received %d",
				label, len(devices),
			)
		}

		device := devices[0]
		volumeTag := names.NewVolumeTag(label)
		vol := storage.Volume{
			Tag: volumeTag,
			VolumeInfo: storage.VolumeInfo{
				VolumeId:   volumeTag.String(),
				Size:       device.Size() / humanize.MiByte,
				Persistent: false,
			},
		}
		attachment := storage.VolumeAttachment{
			Volume:  volumeTag,
			Machine: mTag,
			VolumeAttachmentInfo: storage.VolumeAttachmentInfo{
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
