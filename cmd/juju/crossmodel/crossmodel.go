// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The crossmodel command provides an interface that allows to
// manipulate and inspect cross model relation.
package crossmodel

import (
	"github.com/juju/loggo"

	"github.com/juju/juju/api/crossmodel"
	"github.com/juju/juju/cmd/envcmd"
)

var logger = loggo.GetLogger("juju.cmd.juju.crossmodel")

// CrossModelCommandBase is a base structure to get cross model managing client.
type CrossModelCommandBase struct {
	envcmd.EnvCommandBase
}

// NewCrossModelAPI returns a cross model api for the root api endpoint
// that the environment command returns.
func (c *CrossModelCommandBase) NewCrossModelAPI() (*crossmodel.Client, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, err
	}
	return crossmodel.NewClient(root), nil
}
