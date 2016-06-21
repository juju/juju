// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

// These constants define named sources of model config attributes.
// After a call to UpdateModelConfig, any attributes added/removed
// will have a source of JujuModelConfigSource.
const (
	// JujuCloudSource is used to label model config attributes that
	// come from those associated with the host cloud.
	JujuCloudSource = "juju cloud"

	// JujuModelConfigSource is used to label model config attributes that
	// have been explicitly set by the user.
	JujuModelConfigSource = "model"
)
