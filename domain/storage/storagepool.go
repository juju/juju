// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"regexp"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

// defaultStoragePoolLookup is the comparable type for which fixed storage pool
// uuids are stored for.
type defaultStoragePoolLookup struct {
	// N is the name of the storage pool.
	N string

	// T is the provider type of the storage pool.
	T string
}

// StoragePoolOrigin describes the origin source of a storage pool. The primary
// purpose of this value is to distinguish between storage pools that are
// created by users and those that exists within Juju.
type StoragePoolOrigin int

// StoragePoolUUID uniquely identifies a storage pool in the model.
type StoragePoolUUID baseUUID

const (
	// StoragePoolNameMaxLength is the maximum supported length of a storage
	// pool name.
	StoragePoolNameMaxLength = 128

	// StoragePoolNameMinLength is the minimum supported length of a storage
	// pool name.
	StoragePoolNameMinLength = 1
)

// Constants related to [StoragePooolOrigin].
const (
	// StoragePoolOriginUser indicates that the storage pool was created by a
	// user.
	StoragePoolOriginUser StoragePoolOrigin = iota

	// StoragePoolOriginProviderDefault indicates that the storage pool is a
	// default offered by the storage provider.
	StoragePoolOriginProviderDefault
)

var (
	// validStoragePoolNameRegex is the regex pattern used to validate storage
	// pool names when they are being created or updated. A valid storage pool
	// name MUST confirm to the rules of RFC1035 labels, with the exception that
	// we permit and allow longer names up to 126 characters.
	//
	// NOTE: In juju versions prior to 4.0 we only supported the following
	// regex '^[a-zA-Z]+[-?a-zA-Z0-9]*$'. This regex had no upper bounds and
	// allowed for abuse. It also allowed for '?' characters which violate dns
	// naming.
	//
	// It is important that with the change in supported regex the users
	// understands that for storage pools being imported pre a 4.0 source may
	// violate this regex. In this case the pool name should be validated
	// against the old regex for sanity and accepted.
	//
	// See [validStoragePoolNameLegacyRegex] for the older validation regex of
	// storage pool names.
	validStoragePoolNameRegex = regexp.MustCompile("^[[:alpha:]][[:alnum:]-]{0,126}[[:alnum:]]$|^[[:alpha:]]$")
)

// getDefaultStoragePoolUUIDs returns the full set of fixed storage pool uuids
// for the model.
func getDefaultStoragePoolUUIDs() map[defaultStoragePoolLookup]StoragePoolUUID {
	// These fixed values are offered via a static function interface because
	// maps can not be const values in Go. Having this as a variable invites
	// fiddling.
	return map[defaultStoragePoolLookup]StoragePoolUUID{
		{N: "azure", T: "azure"}:           "4270ce4d-83a4-5617-829c-08341166d8f1",
		{N: "azure-premium", T: "azure"}:   "9bc0210e-b115-5f9c-9dc0-398fb51ab5e0",
		{N: "cinder", T: "cinder"}:         "d2fff284-fd78-5939-80e7-85bc5495b144",
		{N: "ebs", T: "ebs"}:               "d332a4e2-ad86-5af1-9f39-c82c4153ae00",
		{N: "ebs-ssd", T: "ebs"}:           "40301372-0198-5081-9bd1-e85d086c1909",
		{N: "gce", T: "gce"}:               "57c79a70-3651-5aa8-a36a-480343dff53f",
		{N: "kubernetes", T: "kubernetes"}: "47052c4e-2955-5768-a053-bf91f52d54e0",
		{N: "loop", T: "loop"}:             "baa26e04-b1f0-50d9-9bf8-4d5a78ffe6ad",
		{N: "lxd", T: "lxd"}:               "16d8c090-8ef4-59b4-8e88-0bc64a0598a3",
		{N: "lxd-btrfs", T: "lxd"}:         "e1acb8b8-c978-5d53-bc22-2a0e7fd58734",
		{N: "lxd-zfs", T: "lxd"}:           "635f1873-be0b-5f07-b841-9fa02466a9f6",
		{N: "maas", T: "maas"}:             "6bbd79bb-ef10-5795-8c33-2a07acd0f6cc",
		{N: "oci", T: "oci"}:               "db714d36-3c0e-56bd-ac82-4e929cc3d60c",
		{N: "iscsi", T: "oci"}:             "d859abf6-4172-58b1-85a9-7aba58b6c364",
		{N: "rootfs", T: "rootfs"}:         "4d9a00e0-bf5f-5823-8ffa-db1a2ffb940c",
		{N: "tmpfs", T: "tmpfs"}:           "6a16b09c-8ca9-5952-a50a-9082ae7c32c1",
	}
}

// GetProviderDefaultStoragePoolUUID returns the fixed storage pool uuid to use
// for a default storage pool.
//
// Use [GenerateProviderDefaultStoragePoolUUIDWithDefaults] if instead of a
// [coreerrors.NotFound] error a new storage pool uuid should be constructed.
//
// The following errors may be expected:
// - coreerrors.NotFound when no storage pool uuid exists for the pool name and
// provider type tuple.
func GetProviderDefaultStoragePoolUUID(
	poolName, providerType string,
) (StoragePoolUUID, error) {
	lookup := defaultStoragePoolLookup{
		N: poolName,
		T: providerType,
	}

	uuid, exists := getDefaultStoragePoolUUIDs()[lookup]
	if !exists {
		return "", errors.Errorf(
			"no default storage pool uuid exists for pool %q of type %q",
			poolName, providerType,
		).Add(coreerrors.NotFound)
	}

	return uuid, nil
}

// GenerateProviderDefaultStoragePoolUUIDWithDefaults returns the fixed storage
// pool uuid to use for a default storage pool. If no fixed storage pool uuid
// exists for the provided information then a new uuid is generated and
// returned.
//
// Use [GetProviderDefaultStoragePoolUUID] if you don't want a new uuid to be
// generated from this call.
func GenerateProviderDefaultStoragePoolUUIDWithDefaults(
	poolName, providerType string,
) (StoragePoolUUID, error) {
	lookup := defaultStoragePoolLookup{
		N: poolName,
		T: providerType,
	}

	uuid, exists := getDefaultStoragePoolUUIDs()[lookup]
	if !exists {
		var err error
		uuid, err = NewStoragePoolUUID()
		if err != nil {
			return "", errors.Errorf("generating new storage pool uuid: %w", err)
		}
	}

	return uuid, nil
}

// IsValidStoragePoolName checks a new or proposed updated storage pool name to
// make sure that it is valid and can be used within a controller. This func
// should only be used for validating storage pool names that are either new or
// being updated. For existing storage pool names that are being imported into a
// model or their history is unknown use [IsValidStoragePoolNameWithLegacy].
//
// For a storage pool name to be valid it MUST meet the rules of RFC1035 with
// the exception that the length is allowed to be 128 characters long.
func IsValidStoragePoolName(name string) bool {
	return len(name) >= StoragePoolNameMinLength &&
		len(name) <= StoragePoolNameMaxLength &&
		validStoragePoolNameRegex.MatchString(name)
}

// NewStoragePoolUUID creates a new, valid storage pool identifier.
func NewStoragePoolUUID() (StoragePoolUUID, error) {
	u, err := newUUID()
	return StoragePoolUUID(u), err
}

// String returns the string representation of this UUID. This function
// satisfies the [fmt.Stringer] interface.
func (u StoragePoolUUID) String() string {
	return baseUUID(u).String()
}

// Validate returns an error if the [StoragePoolUUID] is not valid.
func (u StoragePoolUUID) Validate() error {
	return baseUUID(u).validate()
}
