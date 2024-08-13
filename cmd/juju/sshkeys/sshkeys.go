// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshkeys

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/client/keymanager"
	"github.com/juju/juju/cmd/modelcmd"
)

type SSHKeysBase struct {
	modelcmd.ModelCommandBase
	modelcmd.IAASOnlyCommand

	apiRoot base.APICallCloser
}

// NewKeyManagerClient returns a keymanager client for the root api endpoint
// that the environment command returns.
func (c *SSHKeysBase) NewKeyManagerClient(ctx context.Context) (*keymanager.Client, error) {
	if c.apiRoot == nil {
		var err error
		c.apiRoot, err = c.NewAPIRoot(ctx)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	return keymanager.NewClient(c.apiRoot), nil
}
