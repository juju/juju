// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	apicloud "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
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
	// Local stores whether a client side (aka local) copy is requested.
	Local bool

	// ClientOnly stores whether the command will ONLY operate on a client copy
	// without affecting controller copy.
	ClientOnly bool

	// ControllerOnly stores whether the command will ONLY operate on a controller copy
	// without affecting client copy.
	ControllerOnly bool
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
	f.BoolVar(&c.ClientOnly, "client-only", false, "Client operation only; controller not affected")
	f.BoolVar(&c.ControllerOnly, "controller-only", false, "Controller operation only; client not affected")
	// TODO (juju3) remove me
	f.BoolVar(&c.Local, "local", false, "DEPRECATED (use --client-only): Local operation only; controller not affected")
}

func (c *showCredentialCommand) Init(args []string) error {
	if c.Local && !c.ClientOnly {
		c.ClientOnly = c.Local
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
		Name:    "show-credential",
		Args:    "[<cloud name> <credential name>]",
		Purpose: "Shows credential information stored either on this client or on a controller.",
		Doc:     showCredentialDoc,
		Aliases: []string{"show-credentials"},
	})
}

func (c *showCredentialCommand) Run(ctxt *cmd.Context) error {
	result, err := c.localCredentials(ctxt)
	if err != nil {
		ctxt.Infof("local credential content lookup failed: %v", err)
	}
	all := ControllerCredentials{Local: c.parseContents(ctxt, result)}
	if c.ClientOnly {
		return c.out.Write(ctxt, all)
	}

	remoteContents, err := c.remoteCredentials()
	if err != nil {
		ctxt.Infof("remote credential content lookup failed: %v", err)
	}
	all.Controller = c.parseContents(ctxt, remoteContents)
	if len(all.Local) == 0 && len(all.Controller) == 0 {
		ctxt.Infof("No credentials from this client or from a controller to display.")
		return nil
	}
	return c.out.Write(ctxt, all)
}

func (c *showCredentialCommand) remoteCredentials() ([]params.CredentialContentResult, error) {
	client, err := c.newAPIFunc()
	if err != nil {
		return nil, err
	}

	defer client.Close()

	if v := client.BestAPIVersion(); v < 2 {
		return nil, errors.NotSupportedf("remote credential content lookup in Juju v%d", v)
	}
	remoteContents, err := client.CredentialContents(c.CloudName, c.CredentialName, c.ShowSecrets)
	if err != nil {
		return nil, err
	}
	return remoteContents, nil
}

func (c *showCredentialCommand) localCredentials(ctxt *cmd.Context) ([]params.CredentialContentResult, error) {
	locals, err := credentialsFromLocalCache(c.store, c.CloudName, c.CredentialName)
	if err != nil {
		return nil, err
	}

	if c.CloudName != "" {
		_, ok := locals[c.CloudName]
		if !ok {
			return nil, errors.NotFoundf("locally stored credentials for cloud %q", c.CloudName)
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
	BestAPIVersion() int
	Close() error
}

func (c *showCredentialCommand) NewCredentialAPI() (CredentialContentAPI, error) {
	currentController, err := modelcmd.DetermineCurrentController(c.store)
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
	Local      CloudCredentials `yaml:"local-credentials"`
	Controller CloudCredentials `yaml:"controller-credentials"`
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

To see secrets, content attributes marked as hidden, use --show-secrets option.

To see only credentials from this client, use "--client-only" option.

Examples:

    juju show-credential google my-admin-credential
    juju show-credentials 
    juju show-credentials --client-only
    juju show-credentials --show-secrets

See also: 
    credentials
`
