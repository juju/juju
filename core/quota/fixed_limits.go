// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package quota

const (
	// MaxCharmStateKeySize describes the max allowed key length for each
	// entry that a charm attempts to persist to the controller.
	MaxCharmStateKeySize = 256

	// MaxCharmStateValueSize describes the max allowed value length for
	// each entry that a charm attempts to persist to the controller.
	MaxCharmStateValueSize = 64 * 1024

	// MaxRelationSettingsSize describes the max allowed total size for all
	// key/value pairs in a relation settings collection.
	MaxRelationSettingsSize = 16 * 1024 * 1024
)
