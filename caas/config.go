// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import "strings"

// TODO(caas) - use a broker specific schema and then add tests

// ResourceConfig encapsulates config for CAAS resources like services.
type ResourceConfig map[string]interface{}

const (
	// JujuExternalHostNameKey specifies the hostname of a CAAS application.
	JujuExternalHostNameKey = "juju-external-hostname"

	// JujuApplicationPath specifies the relative http path used to access a CAAS application.
	JujuApplicationPath = "juju-application-path"
)

// Get gets the specified attribute.
func (r ResourceConfig) Get(attrName string, defaultValue interface{}) interface{} {
	if val, ok := r[attrName]; ok {
		return val
	}
	return defaultValue
}

// GetInt gets the specified int attribute.
func (r ResourceConfig) GetInt(attrName string, defaultValue int) int {
	if val, ok := r[attrName]; ok {
		if value, ok := val.(float64); ok {
			return int(value)
		}
		return val.(int)
	}
	return defaultValue
}

// GetString gets the specified string attribute.
func (r ResourceConfig) GetString(attrName string, defaultValue string) string {
	if val, ok := r[attrName]; ok {
		return val.(string)
	}
	return defaultValue
}

// GetStringSlice gets the specified []string attribute.
func (r ResourceConfig) GetStringSlice(attrName string, defaultValue []string) []string {
	if val, ok := r[attrName]; ok {
		return strings.Split(val.(string), ",")
	}
	return defaultValue
}
