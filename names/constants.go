// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package names

const (
	UnitTagPrefix    = "unit-"
	MachineTagPrefix = "machine-"
	ServiceTagPrefix = "service-"
	EnvironTagPrefix = "environment-"
	UserTagPrefix    = "user-"

	ServiceSnippet       = "[a-z][a-z0-9]*(-[a-z0-9]*[a-z][a-z0-9]*)*"
	NumberSnippet        = "(0|[1-9][0-9]*)"
	ContainerSnippet     = "(/[a-z]+/" + NumberSnippet + ")"
	MachineSnippet       = NumberSnippet + ContainerSnippet + "*"
	ContainerSpecSnippet = "(([a-z])*:)?"
)
