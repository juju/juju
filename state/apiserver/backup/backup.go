package backup

import (
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/apiserver/common"
)

type BackupAPI struct {
	st    *state.State
	resrc *common.Resources
	auth  common.Authorizer
}

// NewBackupAPI creates a new server-side FirewallerAPI facade.
func NewBackupAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*BackupAPI, error) {

	return &BackupAPI{
		st:    st,
		resrc: resources,
		auth:  authorizer,
	}, nil
}

// Backup tells the environment to perform a backup, saving state to a file,
// which can then later be used to restore the environment.  This is an
// asynchronous process. Backup will return immediately with the name of the
// file that is being created during this backup process.  The amount of time
// the backup process takes is heavily dependent on the size of the environment.
func (b *BackupAPI) Backup() params.BackupResults {
	return params.BackupResults{}
}

// Restore tells the environment to restore from the given backup file.  Note
// that this is a destructive, irreversible process and should only be performed
// on a newly bootstrapped environment, and cannot be stopped or undone.
func (b *BackupAPI) Restore(p params.Restore) params.RestoreResults {
	return params.RestoreResults{}
}

// List returns the list of backup files that exist on the server, plus the name
// of the file being created by an in-progress backup, if one exists.
func (b *BackupAPI) List() params.BackupListResults {
	return params.BackupListResults{}
}

// Cancel stops the current backup, if one exists, discarding any data saved to
// disk from the backup.
func (b *BackupAPI) Cancel() params.BackupCancelResults {
	return params.BackupCancelResults{}
}

// Download returns the URL of the given backup file, from which it can be
// retrieved.
func (b *BackupAPI) Download(p params.BackupDownload) *params.Error {
	return nil
}
