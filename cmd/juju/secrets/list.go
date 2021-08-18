// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"io"
	"sort"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	apisecrets "github.com/juju/juju/api/secrets"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/core/secrets"
)

type listSecretsCommand struct {
	modelcmd.ModelCommandBase
	out cmd.Output

	listSecretsAPIFunc func() (ListSecretsAPI, error)
	showSecrets        bool
}

var listSecretsDoc = `
Displays the secrets available for charms to use if granted access.

For controller/model admins, the actual secret value is exposed
with the '--show-secrets' option in json or yaml formats.
Secret values are not shown in tabular format.

Examples:

    juju secrets
    juju secrets --format yaml
    juju secrets --format json --show-secrets
`

// ListSecretsAPI is the secrets client API.
type ListSecretsAPI interface {
	ListSecrets(bool) ([]apisecrets.SecretDetails, error)
	Close() error
}

// NewListSecretsCommand returns a command to list secrets metadata.
func NewListSecretsCommand() cmd.Command {
	c := &listSecretsCommand{}
	c.listSecretsAPIFunc = c.secretsAPI

	return modelcmd.Wrap(c)
}

func (c *listSecretsCommand) secretsAPI() (ListSecretsAPI, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apisecrets.NewClient(root), nil

}

// Info implements cmd.Info.
func (c *listSecretsCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "secrets",
		Purpose: "Lists secrets available in the model.",
		Doc:     listSecretsDoc,
		Aliases: []string{"list-secrets"},
	})
}

// SetFlags implements cmd.SetFlags.
func (c *listSecretsCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.showSecrets, "show-secrets", false, "Show secret values, applicable to yaml or json formats only")
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
		"tabular": func(writer io.Writer, value interface{}) error {
			return formatSecretsTabular(writer, value)
		},
	})
}

type secretValueDetails struct {
	Data  secrets.SecretData `json:",omitempty,inline" yaml:",omitempty,inline"`
	Error error              `json:"error,omitempty" yaml:"error,omitempty"`
}

type secretDisplayDetails struct {
	ID          int                 `json:"ID" yaml:"ID"`
	Revision    int                 `json:"revision" yaml:"revision"`
	Path        string              `json:"path" yaml:"path"`
	Scope       secrets.Scope       `json:"scope" yaml:"scope"`
	Version     int                 `json:"version" yaml:"version"`
	Description string              `json:"description,omitempty" yaml:"description,omitempty"`
	Tags        map[string]string   `json:"tags,omitempty" yaml:"tags,omitempty"`
	Provider    string              `json:"backend" yaml:"backend"`
	ProviderID  string              `json:"backend-id,omitempty" yaml:"backend-id,omitempty"`
	CreateTime  time.Time           `json:"create-time" yaml:"create-time"`
	UpdateTime  time.Time           `json:"update-time" yaml:"update-time"`
	Value       *secretValueDetails `json:"value,omitempty" yaml:"value,omitempty"`
}

// Run implements cmd.Run.
func (c *listSecretsCommand) Run(ctxt *cmd.Context) error {
	if c.showSecrets && c.out.Name() == "tabular" {
		ctxt.Infof("secret values are not shown in tabular format")
		c.showSecrets = false
	}

	api, err := c.listSecretsAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer api.Close()

	secrets, err := api.ListSecrets(c.showSecrets)
	if err != nil {
		return errors.Trace(err)
	}
	details := make([]secretDisplayDetails, len(secrets))
	for i, m := range secrets {
		details[i] = secretDisplayDetails{
			Path:        m.Metadata.Path,
			Scope:       m.Metadata.Scope,
			Version:     m.Metadata.Version,
			Description: m.Metadata.Description,
			Tags:        m.Metadata.Tags,
			ID:          m.Metadata.ID,
			Provider:    m.Metadata.Provider,
			ProviderID:  m.Metadata.ProviderID,
			Revision:    m.Metadata.Revision,
			CreateTime:  m.Metadata.CreateTime,
			UpdateTime:  m.Metadata.UpdateTime,
		}
		if c.showSecrets && m.Value != nil {
			details[i].Value = &secretValueDetails{
				Error: m.Error,
			}
			if m.Error != nil {
				continue
			}
			val, err := m.Value.Values()
			if err != nil {
				details[i].Value.Error = err
			} else {
				details[i].Value.Data = val
			}
		}
	}
	return c.out.Write(ctxt, details)
}

// formatSecretsTabular writes a tabular summary of secret information.
func formatSecretsTabular(writer io.Writer, value interface{}) error {
	secrets, ok := value.([]secretDisplayDetails)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", secrets, value)
	}

	tw := output.TabWriter(writer)
	w := output.Wrapper{tw}
	w.SetColumnAlignRight(1)

	w.Println("ID", "Scope", "Revision", "Backend", "Path", "Age")
	sort.Slice(secrets, func(i, j int) bool {
		return secrets[i].Path < secrets[j].Path
	})
	now := time.Now()
	for _, s := range secrets {
		age := common.UserFriendlyDuration(s.UpdateTime, now)
		w.Print(s.ID, s.Scope, s.Revision, s.Provider, s.Path, age)
		w.Println()
	}
	return tw.Flush()
}
