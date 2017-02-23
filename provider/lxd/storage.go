// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/schema"
	"github.com/juju/utils/featureflag"
	"github.com/juju/utils/set"
	"github.com/lxc/lxd"
	"github.com/lxc/lxd/shared/api"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/tools/lxdclient"
)

const (
	lxdStorageProviderType = "lxd"

	// attrLXDStorageDriver is the attribute name for the
	// storage pool's LXD storage driver. This is the only
	// predefined storage attribute; all others are passed
	// on to LXD directly.
	attrLXDStorageDriver = "driver"
)

func (env *environ) storageSupported() bool {
	return featureflag.Enabled(feature.LXDStorage) && env.raw.StorageSupported()
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
	// TODO(axw) copy the rest of the schema from LXD code.
	// Ideally LXD would export the schema over the API, and
	// we would expose it.
}

var lxdStorageConfigChecker = schema.FieldMap(
	lxdStorageConfigFields,
	schema.Defaults{
		attrLXDStorageDriver: "dir",
	},
)

type lxdStorageConfig struct {
	pool   string
	driver string
}

func newLXDStorageConfig(pool string, attrs map[string]interface{}) (*lxdStorageConfig, error) {
	coerced, err := lxdStorageConfigChecker.Coerce(attrs, nil)
	if err != nil {
		return nil, errors.Annotate(err, "validating Azure storage config")
	}
	attrs = coerced.(map[string]interface{})
	driver := attrs[attrLXDStorageDriver].(string)
	lxdStorageConfig := &lxdStorageConfig{
		// TODO(axw) the LXD pool name should probably come from
		// an attribute of the Juju storage pool, rather than the
		// Juju storage pool name directly.
		pool:   pool,
		driver: driver,
		// TODO(axw) other things
	}
	return lxdStorageConfig, nil
}

// ValidateConfig is part of the Provider interface.
func (e *lxdStorageProvider) ValidateConfig(cfg *storage.Config) error {
	_, err := newLXDStorageConfig(cfg.Name(), cfg.Attrs())
	// TODO(axw) sanity check values.
	return errors.Trace(err)
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

// DefaultPools is part of the Provider interface.
func (e *lxdStorageProvider) DefaultPools() []*storage.Config {
	// TODO(axw) other ones
	zfsPool, _ := storage.NewConfig("lxd-zfs", lxdStorageProviderType, map[string]interface{}{
		attrLXDStorageDriver: "zfs",
	})
	return []*storage.Config{zfsPool}
}

// VolumeSource is part of the Provider interface.
func (e *lxdStorageProvider) VolumeSource(cfg *storage.Config) (storage.VolumeSource, error) {
	return nil, errors.NotSupportedf("volumes")
}

// FilesystemSource is part of the Provider interface.
func (e *lxdStorageProvider) FilesystemSource(cfg *storage.Config) (storage.FilesystemSource, error) {
	lxdStorageConfig, err := newLXDStorageConfig(cfg.Name(), cfg.Attrs())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &lxdFilesystemSource{e.env, lxdStorageConfig}, nil
}

type lxdFilesystemSource struct {
	env *environ
	cfg *lxdStorageConfig
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

	// TODO(axw) the filesystem ID needs to be something
	// unique, since the storage pool could potentially be
	// used by something other than Juju.
	volumeName := arg.Tag.String()
	filesystemId := fmt.Sprintf("%s:%s", s.cfg.pool, volumeName)

	config := map[string]string{
	// TODO(axw) for the "dir" driver, the size attribute is rejected
	// by LXD. Ideally LXD would be able to tell us the total size of
	// the filesystem on which the directory was created, though.
	//"size": ...,
	}
	for k, v := range arg.ResourceTags {
		config["user."+k] = v
	}

	// TODO(axw) ensure pool exists

	if err := s.env.raw.VolumeCreate(s.cfg.pool, volumeName, config); err != nil {
		return nil, errors.Trace(err)
	}
	// TODO(axw) handle BadRequest, checking if the volume already exists.

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

func (s *lxdFilesystemSource) filesystemId(v api.StorageVolume) string {
	return fmt.Sprintf("%s:%s", s.cfg.pool, v.Name)
}

// parseFilesystemId parses the given filesystem ID, returning the underlying
// LXD volume name for this source's LXD storage pool.
func (s *lxdFilesystemSource) parseFilesystemId(id string) (string, error) {
	prefix := s.cfg.pool + ":"
	if !strings.HasPrefix(id, prefix) {
		return "", errors.NotValidf("filesystem ID %q", id)
	}
	return id[len(prefix):], nil
}

// ListFilesystems is specified on the storage.FilesystemSource interface.
func (s *lxdFilesystemSource) ListFilesystems() ([]string, error) {
	volumes, err := s.env.raw.VolumeList(s.cfg.pool)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ids := make([]string, len(volumes))
	for i, v := range volumes {
		// TODO(axw) filter volumes?
		ids[i] = s.filesystemId(v)
	}
	return ids, nil
}

// DescribeFilesystems is specified on the storage.FilesystemSource interface.
func (s *lxdFilesystemSource) DescribeFilesystems(filesystemIds []string) ([]storage.DescribeFilesystemsResult, error) {
	volumes, err := s.env.raw.VolumeList(s.cfg.pool)
	if err != nil {
		return nil, errors.Trace(err)
	}
	results := make([]storage.DescribeFilesystemsResult, len(filesystemIds))
	for i, id := range filesystemIds {
		var found bool
		for _, v := range volumes {
			if id != s.filesystemId(v) {
				continue
			}
			// TODO(axw) extract size from properties
			results[i].FilesystemInfo = &storage.FilesystemInfo{
				FilesystemId: id,
			}
			found = true
		}
		if !found {
			results[i].Error = errors.NotFoundf("filesystem %q", id)
		}
	}
	return results, nil
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
	volumeName, err := s.parseFilesystemId(filesystemId)
	if err != nil {
		return errors.Trace(err)
	}
	err = s.env.raw.VolumeDelete(s.cfg.pool, volumeName)
	if err != nil && err != lxd.LXDErrors[http.StatusNotFound] {
		return errors.Trace(err)
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
	instances, err := s.env.Instances(instanceIds)
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

	volumeName, err := s.parseFilesystemId(arg.FilesystemId)
	if err != nil {
		return nil, errors.Trace(err)
	}

	disks := inst.raw.Disks()
	deviceName := arg.Filesystem.String()
	if _, ok := disks[deviceName]; !ok {
		disk := lxdclient.DiskDevice{
			Path:     arg.Path,
			Source:   volumeName,
			Pool:     s.cfg.pool,
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
	instances, err := s.env.Instances(instanceIds)
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
