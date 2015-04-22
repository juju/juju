// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv

import "github.com/juju/utils/featureflag"

const (
	JujuEnvEnvKey           = "JUJU_ENV"
	JujuHomeEnvKey          = "JUJU_HOME"
	JujuRepositoryEnvKey    = "JUJU_REPOSITORY"
	JujuLoggingConfigEnvKey = "JUJU_LOGGING_CONFIG"
	JujuFeatureFlagEnvKey   = "JUJU_DEV_FEATURE_FLAGS"
	// TODO(thumper): 2013-09-02 bug 1219630
	// As much as I'd like to remove JujuContainerType now, it is still
	// needed as MAAS still needs it at this stage, and we can't fix
	// everything at once.
	JujuContainerTypeEnvKey = "JUJU_CONTAINER_TYPE"
	// JujuStatusIsoTimeEnvKey is the env var which if true, will cause status
	// timestamps to be written in RFC3339 format.
	JujuStatusIsoTimeEnvKey = "JUJU_STATUS_ISO_TIME"
)

// FeatureFlags returns a map that can be merged with os.Environ.
func FeatureFlags() map[string]string {
	result := make(map[string]string)
	if envVar := featureflag.AsEnvironmentValue(); envVar != "" {
		result[JujuFeatureFlagEnvKey] = envVar
	}
	return result
}

// MergeEnvironment will return the current environment updated with
// all the values from newValues.  If current is nil, a new map is
// created.  If current is not nil, it is mutated.
func MergeEnvironment(current, newValues map[string]string) map[string]string {
	if current == nil {
		current = make(map[string]string)
	}
	for key, value := range newValues {
		current[key] = value
	}
	return current
}
