// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	apicloud "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

type showCredentialCommand struct {
	modelcmd.CommandBase
	store jujuclient.ClientStore

	out cmd.Output

	newAPIFunc func() (CredentialContentAPI, error)

	CloudName      string
	CredentialName string

	ShowSecrets bool
}

// NewShowCredentialCommand returns a command to show information about
// credentials stored on the controller.
func NewShowCredentialCommand() cmd.Command {
	cmd := &showCredentialCommand{
		store: jujuclient.NewFileClientStore(),
	}
	cmd.newAPIFunc = func() (CredentialContentAPI, error) {
		return cmd.NewCredentialAPI()
	}
	return modelcmd.WrapBase(cmd)
}

func (c *showCredentialCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	// We only support yaml for display purposes.
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
	})
	f.BoolVar(&c.ShowSecrets, "show-secrets", false, "Display credential secret attributes")
}

func (c *showCredentialCommand) Init(args []string) error {
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
		Name:    "show-credential",
		Args:    "[<cloud name> <credential name>]",
		Purpose: "Shows credential information on a controller.",
		Doc:     showCredentialDoc,
		Aliases: []string{"show-credentials"},
	})
}

func (c *showCredentialCommand) Run(ctxt *cmd.Context) error {
	client, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer client.Close()

	if client.BestAPIVersion() < 2 {
		ctxt.Infof("credential content lookup is not supported by this version of Juju")
		return nil
	}
	contents, err := client.CredentialContents(c.CloudName, c.CredentialName, c.ShowSecrets)
	if err != nil {
		ctxt.Infof("Getting credential content failed with: %v", err)
		return err
	}
	return c.parseContents(ctxt, contents)
}

type CredentialContentAPI interface {
	CredentialContents(cloud, credential string, withSecrets bool) ([]params.CredentialContentResult, error)
	BestAPIVersion() int
	Close() error
}

func (c *showCredentialCommand) NewCredentialAPI() (CredentialContentAPI, error) {
	currentController, err := c.store.CurrentController()
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, errors.New("there is no active controller")
		}
		return nil, errors.Trace(err)
	}
	api, err := c.NewAPIRoot(c.store, currentController, "")
	if err != nil {
		return nil, errors.Annotate(err, "opening API connection")
	}
	return apicloud.NewClient(api), nil
}

type CredentialContent struct {
	AuthType   string            `yaml:"auth-type"`
	Attributes map[string]string `yaml:",inline"`
}

type CredentialDetails struct {
	Content CredentialContent `yaml:"content"`
	Models  map[string]string `yaml:"models"`
}

type NamedCredentials map[string]CredentialDetails

type CloudCredentials map[string]NamedCredentials

type ControllerCredentials struct {
	All CloudCredentials `yaml:"controller-credentials"`
}

func (c *showCredentialCommand) parseContents(ctxt *cmd.Context, in []params.CredentialContentResult) error {
	if len(in) == 0 {
		ctxt.Infof("No credential to display")
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
			//cloudGroup = out[info.Content.Cloud]
		}

		models := make(map[string]string, len(info.Models))
		for _, m := range info.Models {
			ownerAccess := m.Access
			if ownerAccess == "" {
				ownerAccess = "no access"
			}
			models[m.Model] = ownerAccess
		}
		out[info.Content.Cloud][info.Content.Name] = CredentialDetails{
			Content: CredentialContent{
				AuthType:   info.Content.AuthType,
				Attributes: info.Content.Attributes,
			},
			Models: models,
		}
	}
	return c.out.Write(ctxt, ControllerCredentials{out})
}

var showCredentialDoc = `
This command displays information about credential(s) stored on the controller
for this user.

To see the contents of a specific credential, supply its cloud and name.
To see all credentials stored for you, supply no arguments.

To see secrets, content attributes marked as hidden, use --show-secrets option.

To see locally stored credentials, use "juju credentials' command.

Examples:

    juju show-credential google my-admin-credential
    juju show-credentials 
    juju show-credentials --show-secrets

See also: 
    credentials
`
