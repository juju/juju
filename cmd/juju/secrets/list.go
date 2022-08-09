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
	"github.com/juju/names/v4"

	apisecrets "github.com/juju/juju/api/client/secrets"
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
	URI            string               `json:"URI" yaml:"URI"`
	Version        int                  `json:"version" yaml:"version"`
	Owner          string               `json:"owner,omitempty" yaml:"owner,omitempty"`
	Provider       string               `json:"backend" yaml:"backend"`
	ProviderID     string               `json:"backend-id,omitempty" yaml:"backend-id,omitempty"`
	Revision       int                  `json:"revision" yaml:"revision"`
	Description    string               `json:"description,omitempty" yaml:"description,omitempty"`
	Label          string               `json:"label,omitempty" yaml:"label,omitempty"`
	RotatePolicy   secrets.RotatePolicy `json:"rotate-policy,omitempty" yaml:"rotate-policy,omitempty"`
	NextRotateTime *time.Time           `json:"next-rotate-time,omitempty" yaml:"next-rotate-time,omitempty"`
	ExpireTime     *time.Time           `json:"expire-time,omitempty" yaml:"expire-time,omitempty"`
	CreateTime     time.Time            `json:"create-time" yaml:"create-time"`
	UpdateTime     time.Time            `json:"update-time" yaml:"update-time"`
	Error          string               `json:"error,omitempty" yaml:"error,omitempty"`
	Value          *secretValueDetails  `json:"value,omitempty" yaml:"value,omitempty"`
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

	result, err := api.ListSecrets(c.showSecrets)
	if err != nil {
		return errors.Trace(err)
	}
	details := make([]secretDisplayDetails, len(result))
	for i, m := range result {
		ownerId := ""
		if owner, err := names.ParseTag(m.Metadata.OwnerTag); err == nil {
			ownerId = owner.Id()
		}
		details[i] = secretDisplayDetails{
			URI:            m.Metadata.URI.ShortString(),
			Version:        m.Metadata.Version,
			Owner:          ownerId,
			Provider:       m.Metadata.Provider,
			ProviderID:     m.Metadata.ProviderID,
			Description:    m.Metadata.Description,
			Label:          m.Metadata.Label,
			RotatePolicy:   m.Metadata.RotatePolicy,
			NextRotateTime: m.Metadata.NextRotateTime,
			ExpireTime:     m.Metadata.ExpireTime,
			Revision:       m.Metadata.Revision,
			CreateTime:     m.Metadata.CreateTime,
			UpdateTime:     m.Metadata.UpdateTime,
			Error:          m.Error,
		}
		if c.showSecrets && m.Value != nil {
			valueDetails := &secretValueDetails{}
			val, err := m.Value.Values()
			if err != nil {
				valueDetails.Error = err
			} else {
				valueDetails.Data = val
			}
			details[i].Value = valueDetails
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

	w.Println("URI", "Revision", "Rotate", "Backend", "Age")
	sort.Slice(secrets, func(i, j int) bool {
		return secrets[i].URI < secrets[j].URI
	})
	now := time.Now()
	for _, s := range secrets {
		age := common.UserFriendlyDuration(s.UpdateTime, now)
		w.Print(s.URI, s.Revision, s.RotatePolicy, s.Provider, age)
		w.Println()
	}
	return tw.Flush()
}
