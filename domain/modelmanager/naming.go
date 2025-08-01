// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import "regexp"

var (
	// validModelName is a regular expression to validate model names.
	validModelName = regexp.MustCompile(`^[a-z0-9]+[a-z0-9-]*$`)
)

// IsValidModelName checks if the provided model name is valid.
func IsValidModelName(modelName string) bool {
	return validModelName.MatchString(modelName)
}
