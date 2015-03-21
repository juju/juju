// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils"

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
	return nil
}

// CreateFilesystems is defined on the FilesystemSource interface.
func (s *tmpfsFilesystemSource) CreateFilesystems(args []storage.FilesystemParams) ([]storage.Filesystem, error) {
	filesystems := make([]storage.Filesystem, 0, len(args))
	for _, arg := range args {
		filesystem, err := s.createFilesystem(arg)
		if err != nil {
			return nil, errors.Annotate(err, "creating filesystem")
		}
		filesystems = append(filesystems, filesystem)
	}
	return filesystems, nil
}

var getpagesize = os.Getpagesize

func (s *tmpfsFilesystemSource) createFilesystem(params storage.FilesystemParams) (storage.Filesystem, error) {
	if err := s.ValidateFilesystemParams(params); err != nil {
		return storage.Filesystem{}, errors.Trace(err)
	}
	// Align size to the page size in MiB.
	sizeInMiB := params.Size
	pageSizeInMiB := uint64(getpagesize()) / (1024 * 1024)
	if pageSizeInMiB > 0 {
		x := (sizeInMiB + pageSizeInMiB - 1)
		sizeInMiB = x - x%pageSizeInMiB
	}

	info := storage.Filesystem{
		Tag:  params.Tag,
		Size: sizeInMiB,
	}

	// Creating the mount is the responsibility of AttachFilesystems.
	// AttachFilesystems needs to know the size so it can pass it onto
	// "mount"; write the size of the filesystem to a file in the
	// storage directory.
	if err := s.writeFilesystemInfo(info); err != nil {
		return storage.Filesystem{}, err
	}

	return info, nil
}

func (s *tmpfsFilesystemSource) AttachFilesystems(args []storage.FilesystemAttachmentParams) ([]storage.FilesystemAttachment, error) {
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

func (s *tmpfsFilesystemSource) attachFilesystem(arg storage.FilesystemAttachmentParams) (storage.FilesystemAttachment, error) {
	path := arg.Path
	if path == "" {
		return storage.FilesystemAttachment{}, errNoMountPoint
	}
	info, err := s.readFilesystemInfo(arg.Filesystem)
	if err != nil {
		return storage.FilesystemAttachment{}, err
	}

	// Check if the mount already exists.
	source, err := df(s.run, path, "source")
	if err != nil {
		return storage.FilesystemAttachment{}, errors.Trace(err)
	}
	if source != arg.Filesystem.String() {
		if err := ensureDir(s.dirFuncs, path); err != nil {
			return storage.FilesystemAttachment{}, err
		}
		if err := ensureEmptyDir(s.dirFuncs, path); err != nil {
			return storage.FilesystemAttachment{}, err
		}
		if _, err := s.run(
			"mount", "-t", "tmpfs", arg.Filesystem.String(), path,
			"-o", fmt.Sprintf("size=%dm", info.Size),
		); err != nil {
			os.Remove(path)
			return storage.FilesystemAttachment{}, errors.Annotate(err, "cannot mount tmpfs")
		}
	}

	return storage.FilesystemAttachment{
		Filesystem: arg.Filesystem,
		Machine:    arg.Machine,
		Path:       path,
	}, nil
}

func (s *tmpfsFilesystemSource) DetachFilesystems(args []storage.FilesystemAttachmentParams) error {
	return errors.NotImplementedf("DetachFilesystems")
}

func (s *tmpfsFilesystemSource) writeFilesystemInfo(info storage.Filesystem) error {
	filename := s.filesystemInfoFile(info.Tag)
	if _, err := os.Stat(filename); err == nil {
		return errors.Errorf("filesystem %v already exists", info.Tag.Id())
	}
	err := utils.WriteYaml(filename, filesystemInfo{&info.Size})
	if err != nil {
		return errors.Annotate(err, "writing filesystem info to disk")
	}
	return err
}

func (s *tmpfsFilesystemSource) readFilesystemInfo(tag names.FilesystemTag) (storage.Filesystem, error) {
	var info filesystemInfo
	if err := utils.ReadYaml(s.filesystemInfoFile(tag), &info); err != nil {
		return storage.Filesystem{}, errors.Annotate(err, "reading filesystem info from disk")
	}
	if info.Size == nil {
		return storage.Filesystem{}, errors.New("invalid filesystem info: missing size")
	}
	return storage.Filesystem{Tag: tag, Size: *info.Size}, nil
}

func (s *tmpfsFilesystemSource) filesystemInfoFile(tag names.FilesystemTag) string {
	return filepath.Join(s.storageDir, tag.Id()+".info")
}

type filesystemInfo struct {
	Size *uint64 `yaml:"size,omitempty"`
}
