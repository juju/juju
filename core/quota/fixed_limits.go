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
)
