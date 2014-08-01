// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

import (
	"time"

	"github.com/juju/juju/version"
)

type BackupStatus string

const (
	StatusAvailable BackupStatus = "available"
	StatusBuilding  BackupStatus = "building"
	StatusStoring   BackupStatus = "storing"
	StatusFailed    BackupStatus = "failed"
	StatusInfoOnly  BackupStatus = "info-only"
)

type BackupInfo struct {
	Name      string `bson:"_id"`
	Timestamp time.Time
	CheckSum  string // SHA-1
	Size      int64
	Version   version.Number
	Status    BackupStatus
}
