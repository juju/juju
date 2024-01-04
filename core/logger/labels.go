// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

// Label represents a common logger label type.
type Label = string

const (
	// HTTP defines a common HTTP request label.
	HTTP Label = "http"

	// METRICS defines a common label for dealing with metric output. This
	// should be used as a fallback for when prometheus isn't available.
	METRICS Label = "metrics"

	// CHARMHUB defines a common label for dealing with the charmhub client
	// and callers.
	CHARMHUB Label = "charmhub"

	// CMR defines a common label for dealing with cross model relations.
	CMR Label = "cmr"

	// CMR_AUTH defines a common label for dealing with cross model relations auth.
	CMR_AUTH Label = "cmr-auth"

	// SECRETS defines a common label for dealing with secrets.
	SECRETS Label = "secrets"

	// WATCHERS defines a common label for dealing with watchers.
	WATCHERS Label = "watchers"

	// MIGRATION defines a common label for dealing with migration.
	MIGRATION Label = "migration"
)
