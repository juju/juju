// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"fmt"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	apicloud "github.com/juju/juju/api/client/cloud"
	"github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
)

type showCredentialCommand struct {
	modelcmd.OptionalControllerCommand

	out cmd.Output

	newAPIFunc func() (CredentialContentAPI, error)

	CloudName      string
	CredentialName string

	ShowSecrets bool
}

// NewShowCredentialCommand returns a command to show information about
// credentials stored on the controller.
func NewShowCredentialCommand() cmd.Command {
	store := jujuclient.NewFileClientStore()
	command := &showCredentialCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{
			Store:    store,
			ReadOnly: true,
		},
	}
	command.newAPIFunc = func() (CredentialContentAPI, error) {
		return command.NewCredentialAPI()
	}
	return modelcmd.WrapBase(command)
}

func (c *showCredentialCommand) SetFlags(f *gnuflag.FlagSet) {
	c.OptionalControllerCommand.SetFlags(f)
	// We only support yaml for display purposes.
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
	})
	f.BoolVar(&c.ShowSecrets, "show-secrets", false, "Display credential secret attributes")
}

func (c *showCredentialCommand) Init(args []string) error {
	if err := c.OptionalControllerCommand.Init(args); err != nil {
		return err
	}
	switch len(args) {
	case 0:
		// will get all credentials stored on the controller for this user.
		break
	case 1:
		return errors.New("both cloud and credential name are needed")
	case 2:
		c.CloudName = args[0]
		c.CredentialName = args[1]
	default:
		return errors.New("only cloud and credential names are supported")
	}
	return nil
}

func (c *showCredentialCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "show-credential",
		Args:     "[<cloud name> <credential name>]",
		Purpose:  "Shows credential information stored either on this client or on a controller.",
		Doc:      showCredentialDoc,
		Aliases:  []string{"show-credentials"},
		Examples: showCredentialExamples,
		SeeAlso: []string{
			"credentials",
			"add-credential",
			"update-credential",
			"remove-credential",
			"autoload-credentials",
		},
	})
}

func (c *showCredentialCommand) Run(ctxt *cmd.Context) error {
	if err := c.MaybePrompt(ctxt, fmt.Sprintf("show credential %q for cloud %q from", c.CredentialName, c.CloudName)); err != nil {
		return errors.Trace(err)
	}
	all := ControllerCredentials{}
	var returnErr error
	if c.Client {
		result, err := c.localCredentials(ctxt)
		if err != nil {
			ctxt.Infof("ERROR client credential content lookup failed: %v", err)
			returnErr = cmd.ErrSilent
		} else {
			all.Client = c.parseContents(ctxt, result)
		}
	}
	if c.ControllerName != "" {
		remoteContents, err := c.remoteCredentials(ctxt)
		if err != nil {
			ctxt.Infof("ERROR credential content lookup on the controller failed: %v", err)
			returnErr = cmd.ErrSilent
		} else {
			all.Controller = c.parseContents(ctxt, remoteContents)
		}
	}
	if len(all.Client) == 0 && len(all.Controller) == 0 {
		ctxt.Infof("No credentials from this client or from a controller to display.")
		return nil
	}
	if err := c.out.Write(ctxt, all); err != nil {
		return err
	}
	return returnErr
}

func (c *showCredentialCommand) remoteCredentials(ctxt *cmd.Context) ([]params.CredentialContentResult, error) {
	client, err := c.newAPIFunc()
	if err != nil {
		return nil, err
	}

	defer client.Close()

	remoteContents, err := client.CredentialContents(c.CloudName, c.CredentialName, c.ShowSecrets)
	if err != nil {
		return nil, err
	}
	return remoteContents, nil
}

func (c *showCredentialCommand) localCredentials(ctxt *cmd.Context) ([]params.CredentialContentResult, error) {
	locals, err := credentialsFromLocalCache(c.Store, c.CloudName, c.CredentialName)
	if err != nil {
		return nil, err
	}

	if c.CloudName != "" {
		_, ok := locals[c.CloudName]
		if !ok {
			return nil, errors.NotFoundf("client credentials for cloud %q", c.CloudName)
		}
	}

	result := []params.CredentialContentResult{}
	for cloudName, cloudLocal := range locals {
		if !c.ShowSecrets {
			if err := removeSecrets(cloudName, &cloudLocal, cloud.CloudByName); err != nil {
				ctxt.Warningf("removing secrets from credentials for cloud %v: %v", c.CloudName, err)
				continue
			}
		}

		for name, details := range cloudLocal.AuthCredentials {
			result = append(result, params.CredentialContentResult{
				Result: &params.ControllerCredentialInfo{
					Content: params.CredentialContent{
						Name:       name,
						Cloud:      cloudName,
						AuthType:   string(details.AuthType()),
						Attributes: details.Attributes(),
					},
				},
			})
		}
	}
	return result, nil
}

type CredentialContentAPI interface {
	CredentialContents(cloud, credential string, withSecrets bool) ([]params.CredentialContentResult, error)
	Close() error
}

func (c *showCredentialCommand) NewCredentialAPI() (CredentialContentAPI, error) {
	api, err := c.NewAPIRoot(c.Store, c.ControllerName, "")
	if err != nil {
		return nil, errors.Annotate(err, "opening API connection")
	}
	return apicloud.NewClient(api), nil
}

type CredentialContent struct {
	AuthType   string            `yaml:"auth-type"`
	Validity   string            `yaml:"validity-check,omitempty"`
	Attributes map[string]string `yaml:",inline"`
}

type CredentialDetails struct {
	Content CredentialContent `yaml:"content"`
	Models  map[string]string `yaml:"models,omitempty"`
}

type NamedCredentials map[string]CredentialDetails

type CloudCredentials map[string]NamedCredentials

type ControllerCredentials struct {
	Controller CloudCredentials `yaml:"controller-credentials,omitempty"`
	Client     CloudCredentials `yaml:"client-credentials,omitempty"`
}

func (c *showCredentialCommand) parseContents(ctxt *cmd.Context, in []params.CredentialContentResult) CloudCredentials {
	if len(in) == 0 {
		return nil
	}

	out := CloudCredentials{}
	for _, result := range in {
		if result.Error != nil {
			ctxt.Infof("%v", result.Error)
			continue
		}

		info := result.Result
		_, ok := out[info.Content.Cloud]
		if !ok {
			out[info.Content.Cloud] = NamedCredentials{}
		}

		models := make(map[string]string, len(info.Models))
		for _, m := range info.Models {
			ownerAccess := m.Access
			if ownerAccess == "" {
				ownerAccess = "no access"
			}
			models[m.Model] = ownerAccess
		}
		valid := ""
		if info.Content.Valid != nil {
			valid = common.HumanReadableBoolPointer(info.Content.Valid, "valid", "invalid")
		}

		out[info.Content.Cloud][info.Content.Name] = CredentialDetails{
			Content: CredentialContent{
				AuthType:   info.Content.AuthType,
				Attributes: info.Content.Attributes,
				Validity:   valid,
			},
			Models: models,
		}
	}
	return out
}

var showCredentialDoc = `
This command displays information about cloud credential(s) stored
either on this client or on a controller for this user.

To see the contents of a specific credential, supply its cloud and name.
To see all credentials stored for you, supply no arguments.

To see secrets, content attributes marked as hidden, use the ` + "`--show-secrets`" + ` option.

To see credentials from this client, use the ` + "`--client`" + ` option.

To see credentials from a controller, use the ` + "`--controller`" + ` option.
`

const showCredentialExamples = `
    juju show-credential google my-admin-credential
    juju show-credentials
    juju show-credentials --controller mycontroller --client
    juju show-credentials --controller mycontroller
    juju show-credentials --client
    juju show-credentials --show-secrets
`
