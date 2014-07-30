// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"fmt"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/state/backup/api"
)

func (c *Client) Backup(args api.BackupArgs) (p api.BackupResult, err error) {
	backupAPI, err := api.NewBackupServerAPI(c.api.state)
	if err != nil {
		return p, err
	}
	result, err := api.HandleRequest(backupAPI, &args)
	if err != nil {
		return p, err
	}
	p = *result
	return p, nil
}
