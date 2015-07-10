// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/storage"
)

const (
	RootfsProviderType = storage.ProviderType("rootfs")
)

// rootfsProviders create storage sources which provide access to filesystems.
type rootfsProvider struct {
	// run is a function type used for running commands on the local machine.
	run runCommandFunc
}

var (
	_ storage.Provider = (*rootfsProvider)(nil)
)

// ValidateConfig is defined on the Provider interface.
func (p *rootfsProvider) ValidateConfig(cfg *storage.Config) error {
	// Rootfs provider has no configuration.
	return nil
}

// validateFullConfig validates a fully-constructed storage config,
// combining the user-specified config and any internally specified
// config.
func (p *rootfsProvider) validateFullConfig(cfg *storage.Config) error {
	if err := p.ValidateConfig(cfg); err != nil {
		return err
	}
	storageDir, ok := cfg.ValueString(storage.ConfigStorageDir)
	if !ok || storageDir == "" {
		return errors.New("storage directory not specified")
	}
	return nil
}

// VolumeSource is defined on the Provider interface.
func (p *rootfsProvider) VolumeSource(environConfig *config.Config, providerConfig *storage.Config) (storage.VolumeSource, error) {
	return nil, errors.NotSupportedf("volumes")
}

// FilesystemSource is defined on the Provider interface.
func (p *rootfsProvider) FilesystemSource(environConfig *config.Config, sourceConfig *storage.Config) (storage.FilesystemSource, error) {
	if err := p.validateFullConfig(sourceConfig); err != nil {
		return nil, err
	}
	// storageDir is validated by validateFullConfig.
	storageDir, _ := sourceConfig.ValueString(storage.ConfigStorageDir)
	return &rootfsFilesystemSource{
		&osDirFuncs{p.run},
		p.run,
		storageDir,
	}, nil
}

// Supports is defined on the Provider interface.
func (*rootfsProvider) Supports(k storage.StorageKind) bool {
	return k == storage.StorageKindFilesystem
}

// Scope is defined on the Provider interface.
func (*rootfsProvider) Scope() storage.Scope {
	return storage.ScopeMachine
}

// Dynamic is defined on the Provider interface.
func (*rootfsProvider) Dynamic() bool {
	return true
}

type rootfsFilesystemSource struct {
	dirFuncs   dirFuncs
	run        runCommandFunc
	storageDir string
}

// ensureDir ensures the specified path is a directory, or
// if it does not exist, that a directory can be created there.
func ensureDir(d dirFuncs, path string) error {
	// If path already exists, we check that it is empty.
	// It is up to the storage provisioner to ensure that any
	// shared storage constraints and attachments with the same
	// path are validated etc. So the check here is more a sanity check.
	fi, err := d.lstat(path)
	if err == nil {
		if !fi.IsDir() {
			return errors.Errorf("path %q must be a directory", path)
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return errors.Trace(err)
	}
	if err := d.mkDirAll(path, 0755); err != nil {
		return errors.Annotate(err, "could not create directory")
	}
	return nil
}

// ensureEmptyDir ensures the specified directory is empty.
func ensureEmptyDir(d dirFuncs, path string) error {
	fileCount, err := d.fileCount(path)
	if err != nil {
		return errors.Annotate(err, "could not read directory")
	}
	if fileCount > 0 {
		return errors.Errorf("%q is not empty", path)
	}
	return nil
}

var _ storage.FilesystemSource = (*rootfsFilesystemSource)(nil)

// ValidateFilesystemParams is defined on the FilesystemSource interface.
func (s *rootfsFilesystemSource) ValidateFilesystemParams(params storage.FilesystemParams) error {
	// ValidateFilesystemParams may be called on a machine other than the
	// machine where the filesystem will be mounted, so we cannot check
	// available size until we get to CreateFilesystem.
	return nil
}

// CreateFilesystems is defined on the FilesystemSource interface.
func (s *rootfsFilesystemSource) CreateFilesystems(args []storage.FilesystemParams) ([]storage.Filesystem, error) {
	filesystems := make([]storage.Filesystem, len(args))
	for i, arg := range args {
		filesystem, err := s.createFilesystem(arg)
		if err != nil {
			return nil, errors.Annotate(err, "creating filesystem")
		}
		filesystems[i] = filesystem
	}
	return filesystems, nil
}

func (s *rootfsFilesystemSource) createFilesystem(params storage.FilesystemParams) (storage.Filesystem, error) {
	var filesystem storage.Filesystem
	if err := s.ValidateFilesystemParams(params); err != nil {
		return filesystem, errors.Trace(err)
	}
	path := filepath.Join(s.storageDir, params.Tag.Id())
	if err := ensureDir(s.dirFuncs, path); err != nil {
		return filesystem, errors.Trace(err)
	}
	if err := ensureEmptyDir(s.dirFuncs, path); err != nil {
		return filesystem, errors.Trace(err)
	}
	sizeInMiB, err := s.dirFuncs.calculateSize(s.storageDir)
	if err != nil {
		os.Remove(path)
		return filesystem, errors.Annotate(err, "getting size")
	}
	if sizeInMiB < params.Size {
		os.Remove(path)
		return filesystem, errors.Errorf("filesystem is not big enough (%dM < %dM)", sizeInMiB, params.Size)
	}
	filesystem = storage.Filesystem{
		params.Tag,
		names.VolumeTag{},
		storage.FilesystemInfo{
			FilesystemId: params.Tag.Id(),
			Size:         sizeInMiB,
		},
	}
	return filesystem, nil
}

// DestroyFilesystems is defined on the FilesystemSource interface.
func (s *rootfsFilesystemSource) DestroyFilesystems(filesystemIds []string) []error {
	// DestroyFilesystems is a no-op; we leave the storage directory
	// in tact for post-mortems and such.
	return make([]error, len(filesystemIds))
}

// AttachFilesystems is defined on the FilesystemSource interface.
func (s *rootfsFilesystemSource) AttachFilesystems(args []storage.FilesystemAttachmentParams) ([]storage.FilesystemAttachment, error) {
	attachments := make([]storage.FilesystemAttachment, len(args))
	for i, arg := range args {
		attachment, err := s.attachFilesystem(arg)
		if err != nil {
			return nil, errors.Annotatef(err, "attaching %s", names.ReadableString(arg.Filesystem))
		}
		attachments[i] = attachment
	}
	return attachments, nil
}

func (s *rootfsFilesystemSource) attachFilesystem(arg storage.FilesystemAttachmentParams) (storage.FilesystemAttachment, error) {
	mountPoint := arg.Path
	if mountPoint == "" {
		return storage.FilesystemAttachment{}, errNoMountPoint
	}
	// The filesystem is created at <storage-dir>/<storage-id>.
	// If it is different to the attachment path, bind mount.
	if err := s.mount(arg.Filesystem, mountPoint); err != nil {
		return storage.FilesystemAttachment{}, err
	}
	return storage.FilesystemAttachment{
		arg.Filesystem,
		arg.Machine,
		storage.FilesystemAttachmentInfo{
			Path: mountPoint,
		},
	}, nil
}

func (s *rootfsFilesystemSource) mount(tag names.FilesystemTag, target string) error {
	fsPath := filepath.Join(s.storageDir, tag.Id())
	if target == fsPath {
		return nil
	}
	logger.Debugf("mounting filesystem %q at %q", fsPath, target)

	if err := ensureDir(s.dirFuncs, target); err != nil {
		return errors.Trace(err)
	}

	mounted, err := s.tryBindMount(fsPath, target)
	if err != nil {
		return errors.Trace(err)
	}
	if mounted {
		return nil
	}
	// We couldn't bind-mount over the designated directory;
	// carry on and check if it's on the same filesystem. If
	// it is, and it's empty, then claim it as our own.

	if err := s.validateSameMountPoints(fsPath, target); err != nil {
		return err
	}

	// The first time we try to take the existing directory, we'll
	// ensure that it's empty and create a file to "claim" it.
	// Future attachments will simply ensure that the claim file
	// exists.
	targetClaimPath := filepath.Join(fsPath, "juju-target-claimed")
	_, err = s.dirFuncs.lstat(targetClaimPath)
	if err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return errors.Trace(err)
	}
	if err := ensureEmptyDir(s.dirFuncs, target); err != nil {
		return errors.Trace(err)
	}
	if err := s.dirFuncs.mkDirAll(targetClaimPath, 0755); err != nil {
		return errors.Annotate(err, "writing claim file")
	}
	return nil
}

func (s *rootfsFilesystemSource) tryBindMount(source, target string) (bool, error) {
	targetSource, err := s.dirFuncs.mountPointSource(target)
	if err != nil {
		return false, errors.Annotate(err, "getting target mount-point source")
	}
	if targetSource == source {
		// Already bind mounted.
		return true, nil
	}
	if err := s.dirFuncs.bindMount(source, target); err != nil {
		logger.Debugf("cannot bind-mount: %v", err)
	} else {
		return true, nil
	}
	return false, nil
}

func (s *rootfsFilesystemSource) validateSameMountPoints(source, target string) error {
	sourceMountPoint, err := s.dirFuncs.mountPoint(source)
	if err != nil {
		return errors.Trace(err)
	}
	targetMountPoint, err := s.dirFuncs.mountPoint(target)
	if err != nil {
		return errors.Trace(err)
	}
	if sourceMountPoint != targetMountPoint {
		return errors.Errorf(
			"%q (%q) and %q (%q) are on different filesystems",
			source, sourceMountPoint, target, targetMountPoint,
		)
	}
	return nil
}

// DetachFilesystems is defined on the FilesystemSource interface.
func (s *rootfsFilesystemSource) DetachFilesystems(args []storage.FilesystemAttachmentParams) error {
	for _, arg := range args {
		if err := maybeUnmount(s.run, s.dirFuncs, arg.Path); err != nil {
			return errors.Annotatef(err, "detaching filesystem %s", arg.Filesystem.Id())
		}
	}
	return nil
}
