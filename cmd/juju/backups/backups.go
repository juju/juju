// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"bytes"
	"io"
	"os"
	"text/template"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/version"

	"github.com/juju/juju/api/backups"
	apiserverbackups "github.com/juju/juju/apiserver/facades/client/backups"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	statebackups "github.com/juju/juju/state/backups"
)

// APIClient represents the backups API client functionality used by
// the backups command.
//
// To regenerate the mocks for the APIClient used by this package,
// run "go generate" from the package directory.
//go:generate go run github.com/golang/mock/mockgen -package backups_test -destination mock_test.go github.com/juju/juju/cmd/juju/backups ArchiveReader,APIClient
type APIClient interface {
	io.Closer
	// Create sends an RPC request to create a new backup.
	Create(notes string, keepCopy, noDownload bool) (*params.BackupsMetadataResult, error)
	// Info gets the backup's metadata.
	Info(id string) (*params.BackupsMetadataResult, error)
	// List gets all stored metadata.
	List() (*params.BackupsListResult, error)
	// Download pulls the backup archive file.
	Download(id string) (io.ReadCloser, error)
	// Upload pushes a backup archive to storage.
	Upload(ar io.ReadSeeker, meta params.BackupsMetadataResult) (string, error)
	// Remove removes the stored backups.
	Remove(ids ...string) ([]params.ErrorResult, error)
	// Restore will restore a backup with the given id into the controller.
	Restore(string, backups.ClientConnection) error
	// RestoreReader will restore a backup file into the controller.
	RestoreReader(io.ReadSeeker, *params.BackupsMetadataResult, backups.ClientConnection) error
}

// CommandBase is the base type for backups sub-commands.
type CommandBase struct {
	modelcmd.ModelCommandBase

	fs      *gnuflag.FlagSet
	verbose bool
	quiet   bool
}

// NewAPIClient returns a client for the backups api endpoint.
func (c *CommandBase) NewAPIClient() (APIClient, error) {
	return newAPIClient(c)
}

// NewAPIClient returns a client for the backups api endpoint.
func (c *CommandBase) NewGetAPI() (APIClient, int, error) {
	return getAPI(c)
}

// SetFlags implements Command.SetFlags.
func (c *CommandBase) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	c.fs = f
}

// Init implements Command.SetFlags.
func (c *CommandBase) Init(args []string) error {
	c.ModelCommandBase.Init(args)
	c.fs.Visit(func(flag *gnuflag.Flag) {
		if flag.Name == "verbose" {
			c.verbose = true
		}
	})

	c.fs.Visit(func(flag *gnuflag.Flag) {
		if flag.Name == "quiet" {
			c.quiet = true
		}
	})
	return nil
}

func (c *CommandBase) validateIaasController(cmdName string) error {
	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}
	return common.ValidateIaasController(c.CommandBase, cmdName, controllerName, c.ClientStore())
}

var newAPIClient = func(c *CommandBase) (APIClient, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return backups.NewClient(root)
}

// GetAPI returns a client and the api version of the controller
var getAPI = func(c *CommandBase) (APIClient, int, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, -1, errors.Trace(err)
	}
	version := root.BestFacadeVersion("Backups")
	client, err := backups.NewClient(root)
	return client, version, errors.Trace(err)
}

// dumpMetadata writes the formatted backup metadata to stdout.
func (c *CommandBase) dumpMetadata(ctx *cmd.Context, result *params.BackupsMetadataResult) {
	ctx.Verbosef(c.metadata(result))
}

const backupMetadataTemplate = `
backup ID:             {{.BackupID}} 
backup format version: {{.FormatVersion}} 
juju version:          {{.JujuVersion}} 
series:                {{.Series}} 

controller UUID:       {{.ControllerUUID}}{{if (gt .HANodes 1)}} 
controllers in HA:     {{.HANodes}}{{end}}
model UUID:            {{.ModelUUID}} 
machine ID:            {{.MachineID}} 
created on host:       {{.Hostname}} 

checksum:              {{.Checksum}} 
checksum format:       {{.ChecksumFormat}} 
size (B):              {{.Size}} 
stored:                {{.Stored}} 
started:               {{.Started}} 
finished:              {{.Finished}} 

notes:                 {{.Notes}} 
`

type MetadataParams struct {
	BackupID       string
	FormatVersion  int64
	Checksum       string
	ChecksumFormat string
	Size           int64
	Stored         time.Time
	Started        time.Time
	Finished       time.Time
	Notes          string
	ControllerUUID string
	HANodes        int64
	ModelUUID      string
	MachineID      string
	Hostname       string
	JujuVersion    version.Number
	Series         string
}

func (c *CommandBase) metadata(result *params.BackupsMetadataResult) string {
	m := MetadataParams{
		result.ID,
		result.FormatVersion,
		result.Checksum,
		result.ChecksumFormat,
		result.Size,
		result.Stored,
		result.Started,
		result.Finished,
		result.Notes,
		result.ControllerUUID,
		result.HANodes,
		result.Model,
		result.Machine,
		result.Hostname,
		result.Version,
		result.Series,
	}
	t := template.Must(template.New("template").Parse(backupMetadataTemplate))
	content := bytes.Buffer{}
	t.Execute(&content, m)
	return content.String()
}

// ArchiveReader can read a backup archive.
//
// To regenerate the mocks for the ArchiveReader used by this package,
// run "go generate" from the package directory.
//go:generate go run github.com/golang/mock/mockgen -package backups_test -destination mock_test.go github.com/juju/juju/cmd/juju/backups ArchiveReader,APIClient
type ArchiveReader interface {
	io.ReadSeeker
	io.Closer
}

var getArchive = func(filename string) (rc ArchiveReader, metaResult *params.BackupsMetadataResult, err error) {
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
	_, err = archive.Seek(0, io.SeekStart)
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
	_, err = archive.Seek(0, io.SeekStart)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	// Pack the metadata into a result.
	// TODO(perrito666) change the identity of ResultfromMetadata to
	// return a pointer.
	mResult := apiserverbackups.CreateResult(meta, "")
	metaResult = &mResult

	return archive, metaResult, nil
}
