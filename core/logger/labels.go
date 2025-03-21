// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

// Tag represents a common logger tag type.
type Tag = string

const (
	// HTTP defines a common HTTP request tag.
	HTTP Tag = "http"

	// METRICS defines a common tag for dealing with metric output. This
	// should be used as a fallback for when prometheus isn't available.
	METRICS Tag = "metrics"

	// CHARMHUB defines a common tag for dealing with the charmhub client
	// and callers.
	CHARMHUB Tag = "charmhub"

	// CMR defines a common tag for dealing with cross model relations.
	CMR Tag = "cmr"

	// CMR_AUTH defines a common tag for dealing with cross model relations auth.
	CMR_AUTH Tag = "cmr-auth"

	// SECRETS defines a common tag for dealing with secrets.
	SECRETS Tag = "secrets"

	// WATCHERS defines a common tag for dealing with watchers.
	WATCHERS Tag = "watchers"

	// MIGRATION defines a common tag for dealing with migration.
	MIGRATION Tag = "migration"

	// OBJECTSTORE defines a common tag for dealing with objectstore.
	OBJECTSTORE Tag = "objectstore"

	// SSHIMPORTER defines a common tag for dealing with ssh key importer.
	SSHIMPORTER Tag = "ssh-importer"

	// STATUS_HISTORY defines a common tag for dealing with status history.
	STATUS_HISTORY Tag = "status-history"

	// DATABASE defines a common tag for dealing with database operations.
	DATABASE Tag = "database"
)
