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
	"github.com/juju/names/v5"

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
	revealSecrets      bool
	owner              string
}

var listSecretsDoc = `
Displays the secrets available for charms to use if granted access.
`

const listSecretsExamples = `
    juju secrets
    juju secrets --format yaml
`

// ListSecretsAPI is the secrets client API.
type ListSecretsAPI interface {
	ListSecrets(bool, secrets.Filter) ([]apisecrets.SecretDetails, error)
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
		Name:     "secrets",
		Purpose:  "Lists secrets available in the model.",
		Doc:      listSecretsDoc,
		Examples: listSecretsExamples,
		Aliases:  []string{"list-secrets"},
	})
}

// SetFlags implements cmd.SetFlags.
func (c *listSecretsCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.owner, "owner", "", "Include secrets for the specified owner")
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

type secretRevisionDetails struct {
	Revision   int        `json:"revision" yaml:"revision"`
	Backend    string     `json:"backend,omitempty" yaml:"backend,omitempty"`
	CreateTime time.Time  `json:"created" yaml:"created"`
	UpdateTime time.Time  `json:"updated" yaml:"updated"`
	ExpireTime *time.Time `json:"expires,omitempty" yaml:"expires,omitempty"`
}

type secretDetailsByID map[string]secretDisplayDetails

type secretDisplayDetails struct {
	URI              *secrets.URI            `json:"-" yaml:"-"`
	LatestRevision   int                     `json:"revision" yaml:"revision"`
	LatestExpireTime *time.Time              `json:"expires,omitempty" yaml:"expires,omitempty"`
	RotatePolicy     secrets.RotatePolicy    `json:"rotation,omitempty" yaml:"rotation,omitempty"`
	NextRotateTime   *time.Time              `json:"rotates,omitempty" yaml:"rotates,omitempty"`
	Owner            string                  `json:"owner,omitempty" yaml:"owner,omitempty"`
	Description      string                  `json:"description,omitempty" yaml:"description,omitempty"`
	Name             string                  `json:"name,omitempty" yaml:"name,omitempty"`
	Label            string                  `json:"label,omitempty" yaml:"label,omitempty"`
	CreateTime       time.Time               `json:"created" yaml:"created"`
	UpdateTime       time.Time               `json:"updated" yaml:"updated"`
	Error            string                  `json:"error,omitempty" yaml:"error,omitempty"`
	Value            *secretValueDetails     `json:"content,omitempty" yaml:"content,omitempty"`
	Revisions        []secretRevisionDetails `json:"revisions,omitempty" yaml:"revisions,omitempty"`
	Access           []AccessInfo            `yaml:"access,omitempty" json:"access,omitempty"`
}

// AccessInfo holds info about a secret access information.
type AccessInfo struct {
	Target string             `json:"target" yaml:"target"`
	Scope  string             `json:"scope" yaml:"scope"`
	Role   secrets.SecretRole `json:"role" yaml:"role"`
}

func toGrantInfo(grants []secrets.AccessInfo) []AccessInfo {
	result := make([]AccessInfo, len(grants))
	for i, grant := range grants {
		result[i] = AccessInfo{
			Target: grant.Target,
			Scope:  grant.Scope,
			Role:   grant.Role,
		}
	}
	return result
}

// Run implements cmd.Run.
func (c *listSecretsCommand) Run(ctxt *cmd.Context) error {
	if c.revealSecrets && c.out.Name() == "tabular" {
		ctxt.Infof("secret values are not shown in tabular format")
		c.revealSecrets = false
	}

	api, err := c.listSecretsAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer api.Close()

	filter := secrets.Filter{}
	if c.owner != "" {
		ownerTag, err := names.ParseTag(c.owner)
		if err != nil {
			return errors.Maskf(err, "invalid owner %q", c.owner)
		}
		owner := ownerTag.String()
		filter.OwnerTag = &owner
	}
	result, err := api.ListSecrets(c.revealSecrets, filter)
	if err != nil {
		return errors.Trace(err)
	}
	details := gatherSecretInfo(result, c.revealSecrets, false, false)
	return c.out.Write(ctxt, details)
}

func gatherSecretInfo(
	secrets []apisecrets.SecretDetails, reveal, includeRevisions, includeGrants bool,
) map[string]secretDisplayDetails {
	details := make(secretDetailsByID)
	for _, m := range secrets {
		ownerId := ""
		name := ""
		label := m.Metadata.Label
		if owner, err := names.ParseTag(m.Metadata.OwnerTag); err == nil {
			ownerId = owner.Id()
			if owner.Kind() == names.ModelTagKind {
				// Model owned (user) secrets have a name, not a label.
				ownerId = "<" + owner.Kind() + ">"
				name = m.Metadata.Label
				label = ""
			}
		}
		info := secretDisplayDetails{
			URI:              m.Metadata.URI,
			Owner:            ownerId,
			LatestRevision:   m.Metadata.LatestRevision,
			LatestExpireTime: m.Metadata.LatestExpireTime,
			Description:      m.Metadata.Description,
			Name:             name,
			Label:            label,
			RotatePolicy:     m.Metadata.RotatePolicy,
			NextRotateTime:   m.Metadata.NextRotateTime,
			CreateTime:       m.Metadata.CreateTime,
			UpdateTime:       m.Metadata.UpdateTime,
			Error:            m.Error,
		}
		if includeGrants {
			info.Access = toGrantInfo(m.Access)
		}
		if includeRevisions {
			info.Revisions = make([]secretRevisionDetails, len(m.Revisions))
			for i, r := range m.Revisions {
				rev := secretRevisionDetails{
					Revision:   r.Revision,
					CreateTime: r.CreateTime,
					UpdateTime: r.UpdateTime,
					ExpireTime: r.ExpireTime,
				}
				if r.BackendName != nil {
					rev.Backend = *r.BackendName
				}
				info.Revisions[i] = rev
			}
		}
		if reveal && m.Value != nil && !m.Value.IsEmpty() {
			valueDetails := &secretValueDetails{}
			val, err := m.Value.Values()
			if err != nil {
				valueDetails.Error = err
			} else {
				valueDetails.Data = val
			}
			info.Value = valueDetails
		}
		details[info.URI.ID] = info
	}
	return details
}

// formatSecretsTabular writes a tabular summary of secret information.
func formatSecretsTabular(writer io.Writer, value interface{}) error {
	result, ok := value.(map[string]secretDisplayDetails)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", result, value)
	}

	var secrets []secretDisplayDetails
	for _, s := range result {
		secrets = append(secrets, s)
	}

	tw := output.TabWriter(writer)
	w := output.Wrapper{tw}
	w.SetColumnAlignRight(4)

	w.Println("ID", "Name", "Owner", "Rotation", "Revision", "Last updated")
	sort.Slice(secrets, func(i, j int) bool {
		if secrets[i].Owner != secrets[j].Owner {
			return secrets[i].Owner < secrets[j].Owner
		}
		return secrets[i].LatestRevision > secrets[j].LatestRevision
	})
	now := time.Now()
	for _, s := range secrets {
		name := s.Name
		if name == "" {
			name = "-"
		}
		age := common.UserFriendlyDuration(s.UpdateTime, now)
		w.Print(s.URI.ID, name, s.Owner, s.RotatePolicy, s.LatestRevision, age)
		w.Println()
	}
	return tw.Flush()
}
