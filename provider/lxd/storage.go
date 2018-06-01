// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/schema"
	"github.com/lxc/lxd/shared"
	"github.com/lxc/lxd/shared/api"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/tools/lxdclient"
)

const (
	lxdStorageProviderType = "lxd"

	// attrLXDStorageDriver is the attribute name for the
	// storage pool's LXD storage driver. This and "lxd-pool"
	// are the only predefined storage attributes; all others
	// are passed on to LXD directly.
	attrLXDStorageDriver = "driver"

	// attrLXDStoragePool is the attribute name for the
	// storage pool's corresponding LXD storage pool name.
	// If this is not provided, the LXD storage pool name
	// will be set to "juju".
	attrLXDStoragePool = "lxd-pool"
)

func (env *environ) storageSupported() bool {
	return env.raw.StorageSupported()
}

// StorageProviderTypes implements storage.ProviderRegistry.
func (env *environ) StorageProviderTypes() ([]storage.ProviderType, error) {
	var types []storage.ProviderType
	if env.storageSupported() {
		types = append(types, lxdStorageProviderType)
	}
	return types, nil
}

// StorageProvider implements storage.ProviderRegistry.
func (env *environ) StorageProvider(t storage.ProviderType) (storage.Provider, error) {
	if env.storageSupported() && t == lxdStorageProviderType {
		return &lxdStorageProvider{env}, nil
	}
	return nil, errors.NotFoundf("storage provider %q", t)
}

// lxdStorageProvider is a storage provider for LXD volumes, exposed to Juju as
// filesystems.
type lxdStorageProvider struct {
	env *environ
}

var _ storage.Provider = (*lxdStorageProvider)(nil)

var lxdStorageConfigFields = schema.Fields{
	attrLXDStorageDriver: schema.OneOf(
		schema.Const("zfs"),
		schema.Const("dir"),
		schema.Const("btrfs"),
		schema.Const("lvm"),
	),
	attrLXDStoragePool: schema.String(),
}

var lxdStorageConfigChecker = schema.FieldMap(
	lxdStorageConfigFields,
	schema.Defaults{
		attrLXDStorageDriver: "dir",
		attrLXDStoragePool:   schema.Omit,
	},
)

type lxdStorageConfig struct {
	lxdPool string
	driver  string
	attrs   map[string]string
}

func newLXDStorageConfig(attrs map[string]interface{}) (*lxdStorageConfig, error) {
	coerced, err := lxdStorageConfigChecker.Coerce(attrs, nil)
	if err != nil {
		return nil, errors.Annotate(err, "validating Azure storage config")
	}
	attrs = coerced.(map[string]interface{})

	driver := attrs[attrLXDStorageDriver].(string)
	lxdPool, _ := attrs[attrLXDStoragePool].(string)
	delete(attrs, attrLXDStorageDriver)
	delete(attrs, attrLXDStoragePool)

	var stringAttrs map[string]string
	if len(attrs) > 0 {
		stringAttrs = make(map[string]string)
		for k, v := range attrs {
			if vString, ok := v.(string); ok {
				stringAttrs[k] = vString
			} else {
				stringAttrs[k] = fmt.Sprint(v)
			}
		}
	}

	if lxdPool == "" {
		lxdPool = "juju"
	}

	lxdStorageConfig := &lxdStorageConfig{
		lxdPool: lxdPool,
		driver:  driver,
		attrs:   stringAttrs,
	}
	return lxdStorageConfig, nil
}

// ValidateConfig is part of the Provider interface.
func (e *lxdStorageProvider) ValidateConfig(cfg *storage.Config) error {
	lxdStorageConfig, err := newLXDStorageConfig(cfg.Attrs())
	if err != nil {
		return errors.Trace(err)
	}
	return ensureLXDStoragePool(e.env, lxdStorageConfig)
}

// Supports is part of the Provider interface.
func (e *lxdStorageProvider) Supports(k storage.StorageKind) bool {
	return k == storage.StorageKindFilesystem
}

// Scope is part of the Provider interface.
func (e *lxdStorageProvider) Scope() storage.Scope {
	return storage.ScopeEnviron
}

// Dynamic is part of the Provider interface.
func (e *lxdStorageProvider) Dynamic() bool {
	return true
}

// Releasable is defined on the Provider interface.
func (*lxdStorageProvider) Releasable() bool {
	return true
}

// DefaultPools is part of the Provider interface.
func (e *lxdStorageProvider) DefaultPools() []*storage.Config {
	zfsPool, _ := storage.NewConfig("lxd-zfs", lxdStorageProviderType, map[string]interface{}{
		attrLXDStorageDriver: "zfs",
		attrLXDStoragePool:   "juju-zfs",
		"zfs.pool_name":      "juju-lxd",
	})
	btrfsPool, _ := storage.NewConfig("lxd-btrfs", lxdStorageProviderType, map[string]interface{}{
		attrLXDStorageDriver: "btrfs",
		attrLXDStoragePool:   "juju-btrfs",
	})

	var pools []*storage.Config
	if e.ValidateConfig(zfsPool) == nil {
		pools = append(pools, zfsPool)
	}
	if e.ValidateConfig(btrfsPool) == nil {
		pools = append(pools, btrfsPool)
	}
	return pools
}

// VolumeSource is part of the Provider interface.
func (e *lxdStorageProvider) VolumeSource(cfg *storage.Config) (storage.VolumeSource, error) {
	return nil, errors.NotSupportedf("volumes")
}

// FilesystemSource is part of the Provider interface.
func (e *lxdStorageProvider) FilesystemSource(cfg *storage.Config) (storage.FilesystemSource, error) {
	return &lxdFilesystemSource{e.env}, nil
}

func ensureLXDStoragePool(env *environ, cfg *lxdStorageConfig) error {
	createErr := env.raw.CreateStoragePool(cfg.lxdPool, cfg.driver, cfg.attrs)
	if createErr == nil {
		return nil
	}
	// There's no specific error to check for, so we just assume
	// that the error is due to the pool already existing, and
	// verify that. If it doesn't exist, return the original
	// CreateStoragePool error.

	pool, err := env.raw.StoragePool(cfg.lxdPool)
	if errors.IsNotFound(err) {
		return errors.Annotatef(createErr, "creating LXD storage pool %q", cfg.lxdPool)
	} else if err != nil {
		return errors.Annotatef(createErr, "getting storage pool %q", cfg.lxdPool)
	}
	// The storage pool already exists: check that the existing pool's
	// driver and config match what we want.
	if pool.Driver != cfg.driver {
		return errors.Errorf(
			`LXD storage pool %q exists, with conflicting driver %q. Specify an alternative pool name via the "lxd-pool" attribute.`,
			pool.Name, pool.Driver,
		)
	}
	for k, v := range cfg.attrs {
		if haveV, ok := pool.Config[k]; !ok || haveV != v {
			return errors.Errorf(
				`LXD storage pool %q exists, with conflicting config attribute %q=%q. Specify an alternative pool name via the "lxd-pool" attribute.`,
				pool.Name, k, haveV,
			)
		}
	}
	return nil
}

type lxdFilesystemSource struct {
	env *environ
}

// CreateFilesystems is specified on the storage.FilesystemSource interface.
func (s *lxdFilesystemSource) CreateFilesystems(args []storage.FilesystemParams) (_ []storage.CreateFilesystemsResult, err error) {
	results := make([]storage.CreateFilesystemsResult, len(args))
	for i, arg := range args {
		if err := s.ValidateFilesystemParams(arg); err != nil {
			results[i].Error = err
			continue
		}
		filesystem, err := s.createFilesystem(arg)
		if err != nil {
			results[i].Error = err
			continue
		}
		results[i].Filesystem = filesystem
	}
	return results, nil
}

func (s *lxdFilesystemSource) createFilesystem(
	arg storage.FilesystemParams,
) (*storage.Filesystem, error) {

	cfg, err := newLXDStorageConfig(arg.Attributes)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := ensureLXDStoragePool(s.env, cfg); err != nil {
		return nil, errors.Trace(err)
	}

	// The filesystem ID needs to be something unique, since there
	// could be multiple models creating volumes within the same
	// LXD storage pool.
	volumeName := s.env.namespace.Value(arg.Tag.String())
	filesystemId := makeFilesystemId(cfg, volumeName)

	config := map[string]string{}
	for k, v := range arg.ResourceTags {
		config["user."+k] = v
	}
	switch cfg.driver {
	case "dir":
		// NOTE(axw) for the "dir" driver, the size attribute is rejected
		// by LXD. Ideally LXD would be able to tell us the total size of
		// the filesystem on which the directory was created, though.
	default:
		config["size"] = fmt.Sprintf("%dMB", arg.Size)
	}

	if err := s.env.raw.VolumeCreate(cfg.lxdPool, volumeName, config); err != nil {
		return nil, errors.Annotate(err, "creating volume")
	}

	filesystem := storage.Filesystem{
		arg.Tag,
		names.VolumeTag{},
		storage.FilesystemInfo{
			FilesystemId: filesystemId,
			Size:         arg.Size,
		},
	}
	return &filesystem, nil
}

func makeFilesystemId(cfg *lxdStorageConfig, volumeName string) string {
	// We need to include the LXD pool name in the filesystem ID,
	// so that we can map it back to a volume.
	return fmt.Sprintf("%s:%s", cfg.lxdPool, volumeName)
}

// parseFilesystemId parses the given filesystem ID, returning the underlying
// LXD storage pool name and volume name.
func parseFilesystemId(id string) (lxdPool, volumeName string, _ error) {
	fields := strings.SplitN(id, ":", 2)
	if len(fields) < 2 {
		return "", "", errors.Errorf(
			"invalid filesystem ID %q; expected ID in format <lxd-pool>:<volume-name>", id,
		)
	}
	return fields[0], fields[1], nil
}

func destroyControllerFilesystems(env *environ, controllerUUID string) error {
	return errors.Trace(destroyFilesystems(env, func(v api.StorageVolume) bool {
		return v.Config["user."+tags.JujuController] == env.Config().UUID()
	}))
}

func destroyModelFilesystems(env *environ) error {
	return errors.Trace(destroyFilesystems(env, func(v api.StorageVolume) bool {
		return v.Config["user."+tags.JujuModel] == env.Config().UUID()
	}))
}

func destroyFilesystems(env *environ, match func(api.StorageVolume) bool) error {
	pools, err := env.raw.StoragePools()
	if err != nil {
		return errors.Annotate(err, "listing LXD storage pools")
	}
	for _, pool := range pools {
		volumes, err := env.raw.VolumeList(pool.Name)
		if err != nil {
			return errors.Annotatef(err, "listing volumes in LXD storage pool %q", pool)
		}
		for _, volume := range volumes {
			if !match(volume) {
				continue
			}
			if err := env.raw.VolumeDelete(pool.Name, volume.Name); err != nil {
				return errors.Annotatef(
					err,
					"deleting volume %q in LXD storage pool %q",
					volume.Name, pool,
				)
			}
		}
	}
	return nil
}

// DestroyFilesystems is specified on the storage.FilesystemSource interface.
func (s *lxdFilesystemSource) DestroyFilesystems(filesystemIds []string) ([]error, error) {
	results := make([]error, len(filesystemIds))
	for i, filesystemId := range filesystemIds {
		results[i] = s.destroyFilesystem(filesystemId)
	}
	return results, nil
}

func (s *lxdFilesystemSource) destroyFilesystem(filesystemId string) error {
	poolName, volumeName, err := parseFilesystemId(filesystemId)
	if err != nil {
		return errors.Trace(err)
	}
	err = s.env.raw.VolumeDelete(poolName, volumeName)
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	return nil
}

// ReleaseFilesystems is specified on the storage.FilesystemSource interface.
func (s *lxdFilesystemSource) ReleaseFilesystems(filesystemIds []string) ([]error, error) {
	results := make([]error, len(filesystemIds))
	for i, filesystemId := range filesystemIds {
		results[i] = s.releaseFilesystem(filesystemId)
	}
	return results, nil
}

func (s *lxdFilesystemSource) releaseFilesystem(filesystemId string) error {
	poolName, volumeName, err := parseFilesystemId(filesystemId)
	if err != nil {
		return errors.Trace(err)
	}
	volume, err := s.env.raw.Volume(poolName, volumeName)
	if err != nil {
		return errors.Trace(err)
	}
	if volume.Config != nil {
		delete(volume.Config, "user."+tags.JujuModel)
		delete(volume.Config, "user."+tags.JujuController)
		if err := s.env.raw.VolumeUpdate(poolName, volumeName, volume); err != nil {
			return errors.Annotatef(
				err, "removing tags from volume %q in pool %q",
				volumeName, poolName,
			)
		}
	}
	return nil
}

// ValidateFilesystemParams is specified on the storage.FilesystemSource interface.
func (s *lxdFilesystemSource) ValidateFilesystemParams(params storage.FilesystemParams) error {
	// TODO(axw) sanity check params
	return nil
}

// AttachFilesystems is specified on the storage.FilesystemSource interface.
func (s *lxdFilesystemSource) AttachFilesystems(args []storage.FilesystemAttachmentParams) ([]storage.AttachFilesystemsResult, error) {
	var instanceIds []instance.Id
	instanceIdsSeen := make(set.Strings)
	for _, arg := range args {
		if instanceIdsSeen.Contains(string(arg.InstanceId)) {
			continue
		}
		instanceIdsSeen.Add(string(arg.InstanceId))
		instanceIds = append(instanceIds, arg.InstanceId)
	}
	instances, err := s.env.Instances(context.NewCloudCallContext(), instanceIds)
	switch err {
	case nil, environs.ErrPartialInstances, environs.ErrNoInstances:
	default:
		return nil, errors.Trace(err)
	}

	results := make([]storage.AttachFilesystemsResult, len(args))
	for i, arg := range args {
		var inst *environInstance
		for i, instanceId := range instanceIds {
			if instanceId != arg.InstanceId {
				continue
			}
			if instances[i] != nil {
				inst = instances[i].(*environInstance)
			}
			break
		}
		attachment, err := s.attachFilesystem(arg, inst)
		if err != nil {
			results[i].Error = errors.Annotatef(
				err, "attaching %s to %s",
				names.ReadableString(arg.Filesystem),
				names.ReadableString(arg.Machine),
			)
			continue
		}
		results[i].FilesystemAttachment = attachment
	}
	return results, nil
}

func (s *lxdFilesystemSource) attachFilesystem(
	arg storage.FilesystemAttachmentParams,
	inst *environInstance,
) (*storage.FilesystemAttachment, error) {

	if inst == nil {
		return nil, errors.NotFoundf("instance %q", arg.InstanceId)
	}

	poolName, volumeName, err := parseFilesystemId(arg.FilesystemId)
	if err != nil {
		return nil, errors.Trace(err)
	}

	disks := inst.raw.Disks()
	deviceName := arg.Filesystem.String()
	if _, ok := disks[deviceName]; !ok {
		disk := lxdclient.DiskDevice{
			Path:     arg.Path,
			Source:   volumeName,
			Pool:     poolName,
			ReadOnly: arg.ReadOnly,
		}
		if err := s.env.raw.AttachDisk(inst.raw.Name, deviceName, disk); err != nil {
			return nil, errors.Trace(err)
		}
	}

	filesystemAttachment := storage.FilesystemAttachment{
		arg.Filesystem,
		arg.Machine,
		storage.FilesystemAttachmentInfo{
			Path:     arg.Path,
			ReadOnly: arg.ReadOnly,
		},
	}
	return &filesystemAttachment, nil
}

// DetachFilesystems is specified on the storage.FilesystemSource interface.
func (s *lxdFilesystemSource) DetachFilesystems(args []storage.FilesystemAttachmentParams) ([]error, error) {
	var instanceIds []instance.Id
	instanceIdsSeen := make(set.Strings)
	for _, arg := range args {
		if instanceIdsSeen.Contains(string(arg.InstanceId)) {
			continue
		}
		instanceIdsSeen.Add(string(arg.InstanceId))
		instanceIds = append(instanceIds, arg.InstanceId)
	}
	instances, err := s.env.Instances(context.NewCloudCallContext(), instanceIds)
	switch err {
	case nil, environs.ErrPartialInstances, environs.ErrNoInstances:
	default:
		return nil, errors.Trace(err)
	}

	results := make([]error, len(args))
	for i, arg := range args {
		var inst *environInstance
		for i, instanceId := range instanceIds {
			if instanceId != arg.InstanceId {
				continue
			}
			if instances[i] != nil {
				inst = instances[i].(*environInstance)
			}
			break
		}
		if inst != nil {
			err := s.detachFilesystem(arg, inst)
			results[i] = errors.Annotatef(
				err, "detaching %s",
				names.ReadableString(arg.Filesystem),
			)
		}
	}
	return results, nil
}

func (s *lxdFilesystemSource) detachFilesystem(
	arg storage.FilesystemAttachmentParams,
	inst *environInstance,
) error {
	devices := inst.raw.Disks()
	deviceName := arg.Filesystem.String()
	if _, ok := devices[deviceName]; !ok {
		return nil
	}
	return s.env.raw.RemoveDevice(inst.raw.Name, deviceName)
}

// ImportFilesystem is part of the storage.FilesystemImporter interface.
func (s *lxdFilesystemSource) ImportFilesystem(
	filesystemId string,
	tags map[string]string,
) (storage.FilesystemInfo, error) {
	lxdPool, volumeName, err := parseFilesystemId(filesystemId)
	if err != nil {
		return storage.FilesystemInfo{}, errors.Trace(err)
	}
	volume, err := s.env.raw.Volume(lxdPool, volumeName)
	if err != nil {
		return storage.FilesystemInfo{}, errors.Trace(err)
	}
	if len(volume.UsedBy) > 0 {
		return storage.FilesystemInfo{}, errors.Errorf(
			"filesystem %q is in use by %d containers, cannot import",
			filesystemId, len(volume.UsedBy),
		)
	}

	// NOTE(axw) not all drivers support specifying a volume size.
	// If we can't find a size config attribute, we have to make
	// up a number since the model will not allow a size of zero.
	// We use the magic number 999GiB to indicate that it's unknown.
	size := uint64(999 * 1024) // 999GiB
	if sizeString := volume.Config["size"]; sizeString != "" {
		n, err := shared.ParseByteSizeString(sizeString)
		if err != nil {
			return storage.FilesystemInfo{}, errors.Annotate(err, "parsing size")
		}
		// ParseByteSizeString returns bytes, we want MiB.
		size = uint64(n / (1024 * 1024))
	}

	if len(tags) > 0 {
		// Update the volume's user-data with the given tags. This will
		// include updating the model and controller UUIDs, so that the
		// storage is associated with this controller and model.
		if volume.Config == nil {
			volume.Config = make(map[string]string)
		}
		for k, v := range tags {
			volume.Config["user."+k] = v
		}
		if err := s.env.raw.VolumeUpdate(lxdPool, volumeName, volume); err != nil {
			return storage.FilesystemInfo{}, errors.Annotate(err, "tagging volume")
		}
	}

	return storage.FilesystemInfo{
		FilesystemId: filesystemId,
		Size:         size,
	}, nil
}
