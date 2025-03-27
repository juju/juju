// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"io"

	"github.com/juju/juju/core/semversion"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/tools"
)

var logger = internallogger.GetLogger("juju.agent.tools")

// ToolsManager keeps track of a pool of tools
type ToolsManager interface {

	// ReadTools looks in the current storage to see what tools are
	// available that match the given Binary version.
	ReadTools(version semversion.Binary) (*tools.Tools, error)

	// UnpackTools reads the compressed tarball from the io.Reader and
	// extracts the tools to be used. tools is used to indicate what exact
	// version are in the contents of the tarball
	UnpackTools(tools *tools.Tools, r io.Reader) error
}
