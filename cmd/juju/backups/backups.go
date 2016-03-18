// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"fmt"
	"io"
	"os"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api/backups"
	apiserverbackups "github.com/juju/juju/apiserver/backups"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	statebackups "github.com/juju/juju/state/backups"
)

var backupsDoc = `
"juju backups" is used to manage backups of the state of a juju controller.
Backups are only supported on juju controllers, not hosted models.  For
more information on juju controllers, see:

    juju help juju-controllers
`

const backupsPurpose = "create, manage, and restore backups of juju's state"

// NewSuperCommand returns a new backups super-command.
func NewSuperCommand() cmd.Command {
	backupsCmd := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:        "backups",
		Doc:         backupsDoc,
		UsagePrefix: "juju",
		Purpose:     backupsPurpose,
	})
	backupsCmd.Register(newCreateCommand())
	backupsCmd.Register(newInfoCommand())
	backupsCmd.Register(newListCommand())
	backupsCmd.Register(newDownloadCommand())
	backupsCmd.Register(newUploadCommand())
	backupsCmd.Register(newRemoveCommand())
	backupsCmd.Register(newRestoreCommand())
	return backupsCmd
}

// APIClient represents the backups API client functionality used by
// the backups command.
type APIClient interface {
	io.Closer
	// Create sends an RPC request to create a new backup.
	Create(notes string) (*params.BackupsMetadataResult, error)
	// Info gets the backup's metadata.
	Info(id string) (*params.BackupsMetadataResult, error)
	// List gets all stored metadata.
	List() (*params.BackupsListResult, error)
	// Download pulls the backup archive file.
	Download(id string) (io.ReadCloser, error)
	// Upload pushes a backup archive to storage.
	Upload(ar io.ReadSeeker, meta params.BackupsMetadataResult) (string, error)
	// Remove removes the stored backup.
	Remove(id string) error
	// Restore will restore a backup with the given id into the controller.
	Restore(string, backups.ClientConnection) error
	// RestoreReader will restore a backup file into the controller.
	RestoreReader(io.ReadSeeker, *params.BackupsMetadataResult, backups.ClientConnection) error
}

// CommandBase is the base type for backups sub-commands.
type CommandBase struct {
	// TODO(wallyworld) - remove Log when backup command is flattened.
	Log *cmd.Log
	modelcmd.ModelCommandBase
}

// NewAPIClient returns a client for the backups api endpoint.
func (c *CommandBase) NewAPIClient() (APIClient, error) {
	return newAPIClient(c)
}

// SetFlags implements Command.SetFlags.
func (c *CommandBase) SetFlags(f *gnuflag.FlagSet) {
	if c.Log != nil {
		c.Log.AddFlags(f)
	}
}

var newAPIClient = func(c *CommandBase) (APIClient, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return backups.NewClient(root)
}

// dumpMetadata writes the formatted backup metadata to stdout.
func (c *CommandBase) dumpMetadata(ctx *cmd.Context, result *params.BackupsMetadataResult) {
	fmt.Fprintf(ctx.Stdout, "backup ID:       %q\n", result.ID)
	fmt.Fprintf(ctx.Stdout, "checksum:        %q\n", result.Checksum)
	fmt.Fprintf(ctx.Stdout, "checksum format: %q\n", result.ChecksumFormat)
	fmt.Fprintf(ctx.Stdout, "size (B):        %d\n", result.Size)
	fmt.Fprintf(ctx.Stdout, "stored:          %v\n", result.Stored)

	fmt.Fprintf(ctx.Stdout, "started:         %v\n", result.Started)
	fmt.Fprintf(ctx.Stdout, "finished:        %v\n", result.Finished)
	fmt.Fprintf(ctx.Stdout, "notes:           %q\n", result.Notes)

	fmt.Fprintf(ctx.Stdout, "model ID:        %q\n", result.Model)
	fmt.Fprintf(ctx.Stdout, "machine ID:      %q\n", result.Machine)
	fmt.Fprintf(ctx.Stdout, "created on host: %q\n", result.Hostname)
	fmt.Fprintf(ctx.Stdout, "juju version:    %v\n", result.Version)
}

type readSeekCloser interface {
	io.ReadSeeker
	io.Closer
}

func getArchive(filename string) (rc readSeekCloser, metaResult *params.BackupsMetadataResult, err error) {
	defer func() {
		if err != nil && rc != nil {
			rc.Close()
		}
	}()
	archive, err := os.Open(filename)
	rc = archive
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	// Extract the metadata.
	ad, err := statebackups.NewArchiveDataReader(archive)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	_, err = archive.Seek(0, os.SEEK_SET)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	meta, err := ad.Metadata()
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, nil, errors.Trace(err)
		}
		meta, err = statebackups.BuildMetadata(archive)
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
	}
	// Make sure the file info is set.
	fileMeta, err := statebackups.BuildMetadata(archive)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	if meta.Size() == int64(0) {
		if err := meta.SetFileInfo(fileMeta.Size(), "", ""); err != nil {
			return nil, nil, errors.Trace(err)
		}
	}
	if meta.Checksum() == "" {
		err := meta.SetFileInfo(0, fileMeta.Checksum(), fileMeta.ChecksumFormat())
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
	}
	if meta.Finished == nil || meta.Finished.IsZero() {
		meta.Finished = fileMeta.Finished
	}
	_, err = archive.Seek(0, os.SEEK_SET)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	// Pack the metadata into a result.
	// TODO(perrito666) change the identity of ResultfromMetadata to
	// return a pointer.
	mResult := apiserverbackups.ResultFromMetadata(meta)
	metaResult = &mResult

	return archive, metaResult, nil
}
