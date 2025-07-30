// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// CloudContainer represents the state of a CAAS container, eg pod.
type CloudContainer interface {
	// Unit returns the name of the unit for this container.
	Unit() string

	// ProviderId returns the id assigned to the container/pod
	// by the cloud.
	ProviderId() string

	// Ports returns the open container ports.
	Ports() []string
}

// Containers returns the containers for the specified provider ids.
func (m *CAASModel) Containers(providerIds ...string) ([]CloudContainer, error) {
	return nil, nil
}
