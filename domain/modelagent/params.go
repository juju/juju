// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelagent

import (
	"context"

	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/state/binarystorage"
)

// Storage provides methods for retrieving binary file metadata.
type Storage interface {
	AllMetadata() ([]binarystorage.Metadata, error)
}

// FindAgentsParams defines parameters for the FindAgents method.
type FindAgentsParams struct {
	// Number will be used to match tools versions exactly if non-zero.
	Number semversion.Number

	// MajorVersion will be used to match the major version if non-zero.
	MajorVersion int

	// MinorVersion will be used to match the minor version if non-zero.
	MinorVersion int

	// Arch will be used to match tools by architecture if non-empty.
	Arch string

	// AgentStream will be used to set agent stream to search
	AgentStream string

	// TODO - The following are wired in from the facade caller.
	//        The object store needed for agent storage is the controller store.
	//        The ToolsURLGetter is used across several facades and needs more refactoring.

	// ToolsURLsGetter is used to get the possible URLS for the given agent.
	ToolsURLsGetter func(ctx context.Context, v semversion.Binary) ([]string, error)

	// AgentStorage provides access to agent binary metadata.
	AgentStorage Storage
}
