// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

var (
	GetDefaultEnvironment         = getDefaultEnvironment
	GetCurrentEnvironmentFilePath = getCurrentEnvironmentFilePath
)

// Reset the writers used for Infof and Verbosef
func ResetCommandWriters() {
	infoWriter = nil
	verboseWriter = nil
}
