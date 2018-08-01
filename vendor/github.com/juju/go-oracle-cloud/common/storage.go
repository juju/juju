// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package common

import (
	"errors"
	"fmt"
)

// Index is an index storage number
// that is in the range of 1-10
// this index determines the device name by which the volume
// is exposed to the instance
type Index int

// Validate validates the index provided if it's
// compliant with the oracle cloud storage index system
func (i Index) Validate() (err error) {
	if i < 1 || i > 10 {
		return errors.New(
			"go-oracle-cloud: Invalid storage index number",
		)
	}
	return nil
}

// StateStorage type that holds the state of the storage
// in motion
type StateStorage string

const (
	// StateAttaching describes the storage attachment is in
	// the process of attaching to the instance.
	StateAttaching StateStorage = "attaching"

	// StateAttached describes the storage attachment is
	// attached to the instance.
	StateAttached StateStorage = "attached"

	// StateDetached describes the storage attachment is
	// in the process of detaching from the instance.
	StateDetached StateStorage = "detached"

	// StateUnavailable tells that the storage attachment is unavailable.
	StateUnavailable StateStorage = "unavailable"

	//StateUnknown descibes the state of the storage attachment is not known.
	StateUnknown StateStorage = "unknown"
)

// StoragePool type holds properties for
// storage volumes
type StoragePool string

const (
	// LatencyPool is the for storage volumes that require low latency and high IOPS
	LatencyPool StoragePool = "/oracle/public/storage/latency"
	// DefaultPool is for all other storage volumes, usually the default one
	DefaultPool StoragePool = "/oracle/public/storage/default"
)

// validate used in validating the storage pool
func (s StoragePool) Validate() (err error) {
	if s == "" {
		return errors.New(
			"go-oracle-cloud: Empty storage pool name",
		)
	}

	return nil
}

type StorageSize string

type StorageSizeType string

const (
	B StorageSizeType = "B"
	K StorageSizeType = "K"
	M StorageSizeType = "M"
	G StorageSizeType = "G"
	T StorageSizeType = "T"
)

// NewStorageSize returns a new storage size compliant with the
// oracle storgae spec
func NewStorageSize(n uint64, of StorageSizeType) StorageSize {
	return StorageSize(fmt.Sprintf("%d%s", n, of))
}
