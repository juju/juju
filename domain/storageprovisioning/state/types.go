// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"

	"github.com/juju/juju/domain/life"
)

// attachmentLife represents the current life value of either a filesystem or
// volume attachment in the model.
type attachmentLife struct {
	UUID string    `db:"uuid"`
	Life life.Life `db:"life_id"`
}

// attachmentLives is a convenience type that facilitates transforming a slice
// of [attachmentLives] values to a map.
type attachmentLives []attachmentLife

// Iter provides a seq2 implementation for iterating the values of
// [attachmentLives].
func (l attachmentLives) Iter(yield func(string, life.Life) bool) {
	for _, v := range l {
		if !yield(v.UUID, v.Life) {
			return
		}
	}
}

// attachmentUUID represents the UUID of a storage attachment in the model. This
// is used for either volume or filesystem attachments.
type attachmentUUID struct {
	UUID string `db:"uuid"`
}

// filesystemAttachmentIDs represents the ids of attachment points to a
// filesystem attachment. This information includes the filesystem ID the
// attachment is for. As well as this either the machine or unit name the
// attachment for is included.
type filesystemAttachmentIDs struct {
	UUID         string         `db:"uuid"`
	FilesystemID string         `db:"filesystem_id"`
	MachineName  sql.NullString `db:"machine_name"`
	UnitName     sql.NullString `db:"unit_name"`
}

// filesystemAttachmentUUIDs represents a slice of filesystem attachment UUIDs.
// This type exists so that we can provide sqlair with a named type to process a
// slice of strings.
type filesystemAttachmentUUIDs []string

// filesystemID represents the filesystem id value for a storage filesystem
// instance.
type filesystemID struct {
	ID string `db:"filesystem_id"`
}

// filesystemLife represents the current life value and filesystem id for a
// storage filesystem instance in the model.
type filesystemLife struct {
	ID   string    `db:"filesystem_id"`
	Life life.Life `db:"life_id"`
}

// filesystemLives is a convenience type that facilitates transforming a slice
// of [filesystemLife] values to a map.
type filesystemLives []filesystemLife

// Iter provides a seq2 implementation for iterating the values of
// [filesystemLives].
func (l filesystemLives) Iter(yield func(string, life.Life) bool) {
	for _, v := range l {
		if !yield(v.ID, v.Life) {
			return
		}
	}
}

// machineLife represents the current life value of a machine in the model.
type machineLife struct {
	LifeId int `db:"life_id"`
}

// machineUUID represents the UUID of a record in the machine table.
type machineUUID struct {
	UUID string `db:"uuid"`
}

// unitUUID represents the UUID of a record in the unit table.
type unitUUID struct {
	UUID string `db:"uuid"`
}

// netNodeUUIDRef represents a reference to a network node uuid in a storage
// entity table.
type netNodeUUIDRef struct {
	UUID string `db:"net_node_uuid"`
}

// netNodeUUID represents the UUID of a record in the network node table.
type netNodeUUID struct {
	UUID string `db:"uuid"`
}

// volumeAttachmentIDs represents the ids of attachment points to a
// volume attachment. This information includes the volume ID the
// attachment is for. As well as this either the machine or unit name the
// attachment for is included.
type volumeAttachmentIDs struct {
	UUID        string         `db:"uuid"`
	VolumeID    string         `db:"volume_id"`
	MachineName sql.NullString `db:"machine_name"`
	UnitName    sql.NullString `db:"unit_name"`
}

// volumeAttachmentPlanLife represents the life of a volume attachment plan in
// the model and the volume id for the volume the attachment plan is for.
type volumeAttachmentPlanLife struct {
	VolumeID string    `db:"volume_id"`
	Life     life.Life `db:"life_id"`
}

type volumeAttachmentPlanLives []volumeAttachmentPlanLife

// Iter provides a seq2 implementation for iterating the values of
// [volumeAttachmentPlanLife].
func (l volumeAttachmentPlanLives) Iter(yield func(string, life.Life) bool) {
	for _, v := range l {
		if !yield(v.VolumeID, v.Life) {
			return
		}
	}
}

// volumeAttachmentUUIDs represents a slice of volume attachment UUIDs.
// This type exists so that we can provide sqlair with a named type to process a
// slice of strings.
type volumeAttachmentUUIDs []string

// volumeID represents the volume id value for a storage volume instance.
type volumeID struct {
	ID string `db:"volume_id"`
}

// volumeLife represents the current life value and volume id for a storage
// volume instance in the model.
type volumeLife struct {
	ID   string    `db:"volume_id"`
	Life life.Life `db:"life_id"`
}

// volumeLives is a convenience type that facilitates transforming a slice
// of [volumeLife] values to a map.
type volumeLives []volumeLife

// Iter provides a seq2 implementation for iterating the values of
// [volumeLives].
func (l volumeLives) Iter(yield func(string, life.Life) bool) {
	for _, v := range l {
		if !yield(v.ID, v.Life) {
			return
		}
	}
}

type filesystem struct {
	FilesystemID string           `db:"filesystem_id"`
	VolumeID     sql.Null[string] `db:"volume_id"`
	Size         uint64           `db:"size_mib"`
}

type filesystemAttachment struct {
	FilesystemID string `db:"filesystem_id"`
	MountPoint   string `db:"mount_point"`
	ReadOnly     bool   `db:"read_only"`
}
