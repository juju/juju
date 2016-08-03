// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// MachineRemoval returns information on removed machines that still
// need provider resources to be cleaned up.
type MachineRemoval struct {
	MachineTag       string          `json:"machine-tag"`
	LinkLayerDevices []NetworkConfig `json:"link-layer-devices"`
}

// MachineRemovalsResults holds the result of an API call that returns
// machine removal information.
type MachineRemovalsResults struct {
	Results []MachineRemoval `json:"results"`
}
