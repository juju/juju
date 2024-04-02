// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providertracker

// TrackerType defines the configuration for a provider tracker.
type TrackerType interface {
	// SingularNamespace returns the namespace for the provider tracker or
	// false if the tracker is not singular.
	SingularNamespace() (string, bool)
}

// SingularType returns a new singular type with the given namespace. This
// ensures that the provider tracker only has one tracker.
func SingularType(namespace string) TrackerType {
	return trackerType{namespace: namespace}
}

// MultiType returns a new multi type. This allows the provider tracker to have
// multiple trackers.
func MultiType() TrackerType {
	return trackerType{}
}

type trackerType struct {
	namespace string
}

func (s trackerType) SingularNamespace() (string, bool) {
	return s.namespace, s.namespace != ""
}
