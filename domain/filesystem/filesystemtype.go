// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filesystem

// FilesystemType represents the type of a filesystem
// as recorded in the filesystemtype lookup table.
type FilesystemType int

const (
	Unspecified FilesystemType = iota
	Vfat
	Ext4
	Xfs
	Btrfs
	Zfs
	Jfs
	Squashfs
	Bcachefs
)

const (
	UnspecifiedType = "unspecified"
)
