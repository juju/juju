// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends

import (
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/client/secretbackends"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/output"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/secrets/provider"
)

type listSecretBackendsCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	listSecretBackendsAPIFunc func() (ListSecretBackendsAPI, error)
	revealSecrets             bool
}

var listSecretBackendsDoc = `
Displays the secret backends available for storing secret content.
`

const listSecretBackendsExamples = `
    juju secret-backends
    juju secret-backends --format yaml
`

// ListSecretBackendsAPI is the secrets client API.
type ListSecretBackendsAPI interface {
	ListSecretBackends([]string, bool) ([]secretbackends.SecretBackend, error)
	Close() error
}

// NewListSecretBackendsCommand returns a command to list secrets backends.
func NewListSecretBackendsCommand() cmd.Command {
	c := &listSecretBackendsCommand{}
	c.listSecretBackendsAPIFunc = c.secretBackendsAPI

	return modelcmd.WrapController(c)
}

func (c *listSecretBackendsCommand) secretBackendsAPI() (ListSecretBackendsAPI, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return secretbackends.NewClient(root), nil

}

// Info implements cmd.Info.
func (c *listSecretBackendsCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "secret-backends",
		Purpose:  "Lists secret backends available in the controller.",
		Doc:      listSecretBackendsDoc,
		Aliases:  []string{"list-secret-backends"},
		Examples: listSecretBackendsExamples,
		SeeAlso: []string{
			"add-secret-backend",
			"remove-secret-backend",
			"show-secret-backend",
			"update-secret-backend",
		},
	})
}

// SetFlags implements cmd.SetFlags.
func (c *listSecretBackendsCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.revealSecrets, "reveal", false, "Include sensitive backend config content")
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
		"tabular": func(writer io.Writer, value interface{}) error {
			return formatSecretBackendsTabular(writer, value)
		},
	})
}

type secretBackendsByName map[string]secretBackendDisplayDetails

type secretBackendDisplayDetails struct {
	Name                string               `json:"-" yaml:"-"`
	Backend             string               `json:"backend,omitempty" yaml:"backend,omitempty"`
	TokenRotateInterval *time.Duration       `json:"token-rotate-interval,omitempty" yaml:"token-rotate-interval,omitempty"`
	Config              provider.ConfigAttrs `json:"config,omitempty" yaml:"config,omitempty"`
	NumSecrets          int                  `json:"secrets" yaml:"secrets"`
	Status              status.Status        `json:"status" yaml:"status"`
	Message             string               `json:"message,omitempty" yaml:"message,omitempty"`
	ID                  string               `json:"id,omitempty" yaml:"id,omitempty"`
	Error               string               `json:"error,omitempty" yaml:"error,omitempty"`
}

// Run implements cmd.Run.
func (c *listSecretBackendsCommand) Run(ctxt *cmd.Context) error {
	if c.revealSecrets && c.out.Name() == "tabular" {
		ctxt.Infof("sensitive config values are not shown in tabular format")
		c.revealSecrets = false
	}

	api, err := c.listSecretBackendsAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer api.Close()

	result, err := api.ListSecretBackends(nil, c.revealSecrets)
	if err != nil {
		return errors.Trace(err)
	}
	details := gatherSecretBackendInfo(result)
	if len(details) == 0 {
		ctxt.Infof("no secret backends have been added to this controller\n")
		return nil
	}
	return c.out.Write(ctxt, details)
}

func gatherSecretBackendInfo(backends []secretbackends.SecretBackend) map[string]secretBackendDisplayDetails {
	details := make(secretBackendsByName)
	for _, b := range backends {
		info := secretBackendDisplayDetails{
			Name:                b.Name,
			Backend:             b.BackendType,
			TokenRotateInterval: b.TokenRotateInterval,
			NumSecrets:          b.NumSecrets,
			Status:              b.Status,
			Message:             b.Message,
		}
		// Only display the ID if there's an error or it's an external backend.
		if b.Error != nil {
			info.ID = b.ID
			info.Name = "error-" + b.ID
			info.Status = status.Error
			info.Error = b.Error.Error()
		}
		if !secrets.IsInternalSecretBackendID(b.ID) {
			info.ID = b.ID
		}
		if len(b.Config) > 0 {
			info.Config = make(provider.ConfigAttrs)
			for k, v := range b.Config {
				info.Config[k] = v
			}
		}
		details[info.Name] = info
	}
	return details
}

const (
	ellipsis         = "..."
	maxMessageLength = 80
)

// truncateMessage truncates the given message if it is too long.
func truncateMessage(msg string) string {
	if len(msg) > maxMessageLength {
		return msg[:maxMessageLength-len(ellipsis)] + ellipsis
	}
	return msg
}

// formatSecretBackendsTabular writes a tabular summary of secret information.
func formatSecretBackendsTabular(writer io.Writer, value interface{}) error {
	result, ok := value.(map[string]secretBackendDisplayDetails)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", result, value)
	}

	var backends []secretBackendDisplayDetails
	for _, b := range result {
		backends = append(backends, b)
	}

	tw := output.TabWriter(writer)
	w := output.Wrapper{tw}
	w.SetColumnAlignRight(3)

	w.Println("Name", "Type", "Secrets", "Message")
	sort.Slice(backends, func(i, j int) bool {
		if backends[i].Backend != backends[j].Backend {
			return backends[i].Backend < backends[j].Backend
		}
		return backends[i].Name < backends[j].Name
	})
	for _, b := range backends {
		// Ignore backends where there was an error even fetching it.
		if b.Error != "" {
			continue
		}
		msg := b.Message
		if b.Status != status.Active {
			msg = fmt.Sprintf("%s: %s", b.Status, b.Message)
		}
		w.Print(b.Name, b.Backend, b.NumSecrets, truncateMessage(msg))
		w.Println()
	}
	return tw.Flush()
}
