// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"github.com/juju/juju/state/backup"
)

type Action string

const (
	ActionNoop   Action = "no-op"
	ActionCreate Action = "create"
)

type apiClient interface {
	Backup(args BackupArgs) (BackupResult, error)
}

type BackupAPI interface {
	Create(name string) (info *backup.BackupInfo, url string, err error)
}

func HandleRequest(api BackupAPI, args *BackupArgs) (*BackupResult, error) {
	var result BackupResult

	switch args.Action {
	case ActionNoop:
		// Do nothing.
	case ActionCreate:
		info, url, err := api.Create(args.Name)
		if err != nil {
			return nil, err
		}
		result.Info = *info
		result.URL = url
	default:
		return nil, fmt.Errorf("unsupported backup action: %q", args.Action)
	}

	return &result, nil
}
