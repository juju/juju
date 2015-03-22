// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"
	"os"

	"github.com/juju/errors"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/storage"
)

const (
	TmpfsProviderType = storage.ProviderType("tmpfs")
)

// tmpfsProviders create storage sources which provide access to filesystems.
type tmpfsProvider struct {
	// run is a function type used for running commands on the local machine.
	run runCommandFunc
}

var (
	_ storage.Provider = (*tmpfsProvider)(nil)
)

// ValidateConfig is defined on the Provider interface.
func (p *tmpfsProvider) ValidateConfig(cfg *storage.Config) error {
	// Tmpfs provider has no configuration.
	return nil
}

// validateFullConfig validates a fully-constructed storage config,
// combining the user-specified config and any internally specified
// config.
func (p *tmpfsProvider) validateFullConfig(cfg *storage.Config) error {
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
func (p *tmpfsProvider) VolumeSource(environConfig *config.Config, providerConfig *storage.Config) (storage.VolumeSource, error) {
	return nil, errors.NotSupportedf("volumes")
}

// FilesystemSource is defined on the Provider interface.
func (p *tmpfsProvider) FilesystemSource(environConfig *config.Config, sourceConfig *storage.Config) (storage.FilesystemSource, error) {
	if err := p.validateFullConfig(sourceConfig); err != nil {
		return nil, err
	}
	// storageDir is validated by validateFullConfig.
	storageDir, _ := sourceConfig.ValueString(storage.ConfigStorageDir)

	return &tmpfsFilesystemSource{
		&osDirFuncs{p.run},
		p.run,
		storageDir,
	}, nil
}

// Supports is defined on the Provider interface.
func (*tmpfsProvider) Supports(k storage.StorageKind) bool {
	return k == storage.StorageKindFilesystem
}

// Scope is defined on the Provider interface.
func (*tmpfsProvider) Scope() storage.Scope {
	return storage.ScopeMachine
}

// Dynamic is defined on the Provider interface.
func (*tmpfsProvider) Dynamic() bool {
	return true
}

type tmpfsFilesystemSource struct {
	dirFuncs   dirFuncs
	run        runCommandFunc
	storageDir string
}

var _ storage.FilesystemSource = (*tmpfsFilesystemSource)(nil)

// ValidateFilesystemParams is defined on the FilesystemSource interface.
func (s *tmpfsFilesystemSource) ValidateFilesystemParams(params storage.FilesystemParams) error {
	// ValidateFilesystemParams may be called on a machine other than the
	// machine where the filesystem will be mounted, so we cannot check
	// available size until we get to createFilesystem.
	if params.Attachment == nil {
		return errors.NotSupportedf(
			"creating filesystem without machine attachment",
		)
	}
	return nil
}

// CreateFilesystems is defined on the FilesystemSource interface.
func (s *tmpfsFilesystemSource) CreateFilesystems(args []storage.FilesystemParams,
) ([]storage.Filesystem, []storage.FilesystemAttachment, error) {
	filesystems := make([]storage.Filesystem, 0, len(args))
	filesystemAttachments := make([]storage.FilesystemAttachment, 0, len(args))
	for _, arg := range args {
		filesystem, filesystemAttachment, err := s.createFilesystem(arg)
		if err != nil {
			return nil, nil, errors.Annotate(err, "creating filesystem")
		}
		filesystems = append(filesystems, filesystem)
		filesystemAttachments = append(filesystemAttachments, filesystemAttachment)
	}
	return filesystems, filesystemAttachments, nil

}

func (s *tmpfsFilesystemSource) createFilesystem(params storage.FilesystemParams) (storage.Filesystem, storage.FilesystemAttachment, error) {
	var filesystem storage.Filesystem
	var filesystemAttachment storage.FilesystemAttachment
	if err := s.ValidateFilesystemParams(params); err != nil {
		return filesystem, filesystemAttachment, errors.Trace(err)
	}
	path := params.Attachment.Path
	if path == "" {
		return filesystem, filesystemAttachment, errors.New("cannot create a filesystem mount without specifying a path")
	}
	if err := validatePath(s.dirFuncs, path); err != nil {
		return filesystem, filesystemAttachment, err
	}
	if _, err := s.run(
		"mount", "-t", "tmpfs", "none", path, "-o", fmt.Sprintf("size=%d", params.Size*1024*1024),
	); err != nil {
		os.Remove(path)
		return filesystem, filesystemAttachment, errors.Annotate(err, "cannot mount tmpfs")
	}

	// Just to be sure, we still need to double check the size here because
	//the allocation above is rounded to the nearest page size.
	sizeInMiB, err := s.dirFuncs.calculateSize(path)
	if err != nil {
		os.Remove(path)
		return filesystem, filesystemAttachment, errors.Annotate(err, "getting size")
	}
	if sizeInMiB < params.Size {
		os.Remove(path)
		return filesystem, filesystemAttachment, errors.Errorf("filesystem is not big enough (%dM < %dM)", sizeInMiB, params.Size)
	}

	filesystem = storage.Filesystem{
		Tag:  params.Tag,
		Size: sizeInMiB,
	}
	filesystemAttachment = storage.FilesystemAttachment{
		Filesystem: params.Tag,
		Machine:    params.Attachment.Machine,
		Path:       path,
	}
	return filesystem, filesystemAttachment, nil
}
