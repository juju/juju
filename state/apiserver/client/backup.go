// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"fmt"

	"github.com/juju/juju/state/backup/api"
)

var (
	newBackupAPI = api.NewBackupServerAPI
)

func (c *Client) Backup(args api.Backup) (p api.BackupResult, err error) {
	api, err := newBackupAPI(c.api.state)
	if err != nil {
		return p, err
	}

	switch args.Action {
	case "create":
		info, URL, err := api.Create(args.Name)
		if err != nil {
			return p, err
		}
		p.Info = *info
		p.URL = URL
	default:
		return p, fmt.Errorf("unsupported backup action: %q", args.Action)
	}
	return
}
