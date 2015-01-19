// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import "github.com/juju/juju/instance"

// FeatureFlag is the name of the feature for the JUJU_DEV_FEATURE_FLAGS
// envar. Add this string to the envar to enable support for storage.
const FeatureFlag = "storage"

// DiskParams is a fully specified set of parameters for disk creation,
// derived from one or more of user-specified storage constraints, a
// storage pool definition, and charm storage metadata.
type DiskParams struct {
	// Name is a unique name assigned by Juju for the requested disk.
	Name string

	// Size is the minimum size of the disk in MiB.
	Size uint64

	// Options is a set of provider-specific options for storage creation,
	// as defined in a storage pool.
	Options map[string]interface{}

	// Instance is the ID of the instance that the disk should be attached
	// to initially. This will only be empty if the instance is not yet
	// provisioned, in which case the parameters refer to a disk that is
	// being created in conjunction with the instance.
	Instance instance.Id
}

// ProviderType uniquely identifies a storage provider, such as "ebs" or "loop".
type ProviderType string

// Provider is an interface for obtaining storage sources.
type Provider interface {
	// DiskSource returns a DiskSource given the specified configuration.
	//
	// If the storage provider does not support creating disks as a
	// first-class primitive, then DiskSource must return an error
	// satisfying errors.IsNotSupported.
	DiskSource(*Config) (DiskSource, error)

	// TODO(axw) define filesystem source. If the user requests a
	// filesystem and that can be provided first-class, it should be
	// done that way. Otherwise we create a disk and then manage a
	// filesystem on that.

	// ValidateConfig validates the provided storage provider config,
	// returning an error if it is invalid.
	ValidateConfig(*Config) error
}

// DiskSource provides an interface for creating, destroying and
// describing disks in the environment. A DiskSource is configured
// in a particular way, and corresponds to a storage "pool".
type DiskSource interface {
	// CreateDisks creates disks with the specified size, in MiB.
	CreateDisks(spec []DiskParams) ([]BlockDevice, error)

	// DescribeDisks returns the properties of the disks with the
	// specified provider disk IDs.
	DescribeDisks(diskIds []string) ([]BlockDevice, error)

	// DestroyDisks destroys the disks with the specified provider
	// disk IDs.
	DestroyDisks(diskIds []string) error

	// ValidateDiskParams validates the provided disk parameters,
	// returning an error if they are invalid.
	//
	// If the provider is incapable of provisioning disks separately
	// from machine instances (e.g. MAAS), then ValidateDiskParams
	// must return an error if params.Instance is non-empty.
	ValidateDiskParams(params DiskParams) error

	// AttachDisks attaches the disks with the specified provider
	// disk IDs to the instances with the corresponding index.
	//
	// TODO(axw) we need to validate attachment requests prior to
	// recording in state. For example, the ec2 disk provider must
	// reject an attempt to attach a disk to an instance if they
	// are in different availability zones.
	AttachDisks(diskIds []string, instId []instance.Id) error

	// DetachDisks detaches the disks with the specified provider
	// disk IDs from the instances with the corresponding index.
	//
	// TODO(axw) we need to record in state whether or not disks
	// are detachable, and reject attempts to attach/detach on
	// that basis.
	DetachDisks(diskIds []string, instId []instance.Id) error
}
