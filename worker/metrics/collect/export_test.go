// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package collect

var (
	// NewCollect allows patching the function that creates the metric collection
	// entity.
	NewCollect = &newCollect

	// NewRecorder allows patching the function that creates the metric recorder.
	NewRecorder = &newRecorder
)
