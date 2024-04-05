// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/errors"

	coresecrets "github.com/juju/juju/core/secrets"
)

// These structs represent the persistent secretMetadata entity schema in the database.

type secretMetadata struct {
	ID          string    `db:"id"`
	Version     int       `db:"version"`
	Description string    `db:"description"`
	AutoPrune   bool      `db:"auto_prune"`
	CreateTime  time.Time `db:"create_time"`
	UpdateTime  time.Time `db:"update_time"`
}

// secretInfo is used because sqlair doesn't seem to like struct embedding.
type secretInfo struct {
	ID          string    `db:"id"`
	Version     int       `db:"version"`
	Description string    `db:"description"`
	AutoPrune   bool      `db:"auto_prune"`
	CreateTime  time.Time `db:"create_time"`
	UpdateTime  time.Time `db:"update_time"`

	LatestRevision int `db:"latest_revision"`
}

type secretOwner struct {
	SecretID  string `db:"secret_id"`
	OwnerID   string `db:"owner_id"`
	OwnerKind string `db:"owner_kind"`
	Label     string `db:"label"`
}

type secretRevision struct {
	ID            string    `db:"uuid"`
	SecretID      string    `db:"secret_id"`
	Revision      int       `db:"revision"`
	Obsolete      bool      `db:"obsolete"`
	PendingDelete bool      `db:"pending_delete"`
	CreateTime    time.Time `db:"create_time"`
	UpdateTime    time.Time `db:"update_time"`
}

type secretContent struct {
	// ID holds the cloud uuid.
	ID string `db:"revision_uuid"`

	// Key is the key value.
	Name string `db:"name"`

	// Value is the value associated with key.
	Content string `db:"content"`
}

type secretValueRef struct {
	// ID holds the cloud uuid.
	ID string `db:"revision_uuid"`

	// Key is the key value.
	BackendUUID string `db:"backend_uuid"`

	// Value is the value associated with key.
	RevisionID string `db:"revision_id"`
}

type secrets []secretInfo

func (rows secrets) toSecretMetadata(secretOwners []secretOwner) ([]*coresecrets.SecretMetadata, error) {
	if n := len(rows); n != len(secretOwners) {
		// Should never happen.
		return nil, errors.New("row length mismatch composing secret results")
	}

	result := make([]*coresecrets.SecretMetadata, len(rows))
	for i, row := range rows {
		uri, err := coresecrets.ParseURI(row.ID)
		if err != nil {
			return nil, errors.NotValidf(row.ID)
		}
		result[i] = &coresecrets.SecretMetadata{
			URI:         uri,
			Version:     row.Version,
			Description: row.Description,
			Label:       secretOwners[i].Label,
			Owner: coresecrets.Owner{
				Kind: coresecrets.OwnerKind(secretOwners[i].OwnerKind),
				ID:   secretOwners[i].OwnerID,
			},
			CreateTime:     row.CreateTime,
			UpdateTime:     row.UpdateTime,
			LatestRevision: row.LatestRevision,
			AutoPrune:      row.AutoPrune,
		}
	}
	return result, nil
}

type secretRevisions []secretRevision

func (rows secretRevisions) toSecretRevisions() ([]*coresecrets.SecretRevisionMetadata, error) {
	result := make([]*coresecrets.SecretRevisionMetadata, len(rows))
	for i, row := range rows {
		result[i] = &coresecrets.SecretRevisionMetadata{
			Revision:    row.Revision,
			ValueRef:    nil,
			CreateTime:  row.CreateTime,
			UpdateTime:  row.UpdateTime,
			BackendName: nil,
			ExpireTime:  nil,
		}
	}
	return result, nil
}

type secretValues []secretContent

func (rows secretValues) toSecretData() (coresecrets.SecretData, error) {
	result := make(coresecrets.SecretData)
	for _, row := range rows {
		result[row.Name] = row.Content
	}
	return result, nil
}
