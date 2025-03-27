// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"bytes"
	"context"
	"io"
	"text/template"
	"time"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/client/backups"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/rpc/params"
)

// APIClient represents the backups API client functionality used by
// the backups command.
type APIClient interface {
	io.Closer
	// Create sends an RPC request to create a new backup.
	Create(nctx context.Context, otes string, noDownload bool) (*params.BackupsMetadataResult, error)
	// Download pulls the backup archive file.
	Download(ctx context.Context, filename string) (io.ReadCloser, error)
}

// CommandBase is the base type for backups sub-commands.
type CommandBase struct {
	modelcmd.ModelCommandBase

	fs      *gnuflag.FlagSet
	verbose bool
	quiet   bool
}

// NewAPIClient returns a client for the backups api endpoint.
func (c *CommandBase) NewAPIClient(ctx context.Context) (APIClient, error) {
	return newAPIClient(ctx, c)
}

// NewGetAPI returns a client for the backups api endpoint.
func (c *CommandBase) NewGetAPI(ctx context.Context) (APIClient, error) {
	return getAPI(ctx, c)
}

// SetFlags implements Command.SetFlags.
func (c *CommandBase) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	c.fs = f
}

// Init implements Command.SetFlags.
func (c *CommandBase) Init(args []string) error {
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

func (c *CommandBase) validateIaasController(ctx context.Context, cmdName string) error {
	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}
	return common.ValidateIaasController(ctx, c.CommandBase, cmdName, controllerName, c.ClientStore())
}

var newAPIClient = func(ctx context.Context, c *CommandBase) (APIClient, error) {
	root, err := c.NewAPIRoot(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return backups.NewClient(root), nil
}

// GetAPI returns a client and the api version of the controller
var getAPI = func(ctx context.Context, c *CommandBase) (APIClient, error) {
	root, err := c.NewAPIRoot(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	client := backups.NewClient(root)
	return client, nil
}

const backupMetadataTemplate = `
backup format version: {{.FormatVersion}} 
juju version:          {{.JujuVersion}} 
base:                  {{.Base}} 

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
	JujuVersion    semversion.Number
	Base           string
}

func (c *CommandBase) metadata(result *params.BackupsMetadataResult) string {
	m := MetadataParams{
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
		result.Base,
	}
	t := template.Must(template.New("template").Parse(backupMetadataTemplate))
	content := bytes.Buffer{}
	_ = t.Execute(&content, m)
	return content.String()
}
