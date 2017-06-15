// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"regexp"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils/featureflag"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/application"
	"github.com/juju/juju/api/applicationoffers"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/feature"
)

const addRelationDocCrossModel = `
Add a relation between 2 local application endpoints or a local endpoint and a remote application endpoint.
Adding a relation between two remote application endpoints is not supported.

Application endpoints can be identified either by:
    <application name>[:<relation name>]
        where application name supplied without relation will be internally expanded to be well-formed
or
    <model name>.<application name>[:<relation name>]
        where the application is hosted in another model owned by the current user, in the same controller
or
    <user name>/<model name>.<application name>[:<relation name>]
        where user/model is another model in the same controller

Examples:
    $ juju add-relation wordpress mysql
        where "wordpress" and "mysql" will be internally expanded to "wordpress:db" and "mysql:server" respectively

    $ juju add-relation wordpress someone/prod.mysql
        where "wordpress" will be internally expanded to "wordpress:db"

`

var localEndpointRegEx = regexp.MustCompile("^" + names.RelationSnippet + "$")

// NewAddRelationCommand returns a command to add a relation between 2 services.
func NewAddRelationCommand() cmd.Command {
	return modelcmd.Wrap(&addRelationCommand{})
}

// addRelationCommand adds a relation between two application endpoints.
type addRelationCommand struct {
	modelcmd.ModelCommandBase
	Endpoints         []string
	remoteEndpoint    *crossmodel.ApplicationURL
	addRelationAPI    applicationAddRelationAPI
	consumeDetailsAPI applicationConsumeDetailsAPI
}

func (c *addRelationCommand) Info() *cmd.Info {
	addCmd := &cmd.Info{
		Name:    "add-relation",
		Aliases: []string{"relate"},
		Args:    "<application1>[:<endpoint name1>] <application2>[:<endpoint name2>]",
		Purpose: "Add a relation between two application endpoints.",
	}
	if featureflag.Enabled(feature.CrossModelRelations) {
		addCmd.Doc = addRelationDocCrossModel
	}
	return addCmd
}

func (c *addRelationCommand) Init(args []string) error {
	if len(args) != 2 {
		return errors.Errorf("a relation must involve two applications")
	}
	if err := c.validateEndpoints(args); err != nil {
		return err
	}
	return nil
}

// applicationAddRelationAPI defines the API methods that application add relation command uses.
type applicationAddRelationAPI interface {
	Close() error
	// CHECK
	BestAPIVersion() int
	AddRelation(endpoints ...string) (*params.AddRelationResults, error)
	Consume(crossmodel.ConsumeApplicationArgs) (string, error)
}

func (c *addRelationCommand) getAddRelationAPI() (applicationAddRelationAPI, error) {
	if c.addRelationAPI != nil {
		return c.addRelationAPI, nil
	}

	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return application.NewClient(root), nil
}

func (c *addRelationCommand) getOffersAPI() (applicationConsumeDetailsAPI, error) {
	if c.consumeDetailsAPI != nil {
		return c.consumeDetailsAPI, nil
	}

	controllerName, err := c.ControllerName()
	if err != nil {
		return nil, errors.Trace(err)
	}
	root, err := c.CommandBase.NewAPIRoot(c.ClientStore(), controllerName, "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	return applicationoffers.NewClient(root), nil
}

func (c *addRelationCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAddRelationAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	if c.remoteEndpoint != nil {
		if client.BestAPIVersion() < 3 {
			// old client does not have cross-model capability.
			return errors.NotSupportedf("cannot add relation to %s: remote endpoints", c.remoteEndpoint.String())
		}
		if err := c.maybeConsumeOffer(client); err != nil {
			return errors.Trace(err)
		}
	}

	_, err = client.AddRelation(c.Endpoints...)
	if params.IsCodeUnauthorized(err) {
		common.PermissionsMessage(ctx.Stderr, "add a relation")
	}
	return block.ProcessBlockedError(err, block.BlockChange)
}

func (c *addRelationCommand) maybeConsumeOffer(targetClient applicationAddRelationAPI) error {
	sourceClient, err := c.getOffersAPI()
	if err != nil {
		return errors.Trace(err)
	}
	defer sourceClient.Close()

	// Get the details of the remote offer - this will fail with a permission
	// error if the user isn't authorised to consume the offer.
	consumeDetails, err := sourceClient.GetConsumeDetails(c.remoteEndpoint.String())
	if err != nil {
		return errors.Trace(err)
	}

	// Consume is idempotent so even if the offer has been consumed previously,
	// it's safe to do so again.
	arg := crossmodel.ConsumeApplicationArgs{
		ApplicationOffer: *consumeDetails.Offer,
		ApplicationAlias: c.remoteEndpoint.ApplicationName,
		Macaroon:         consumeDetails.Macaroon,
	}
	if consumeDetails.ControllerInfo != nil {
		controllerTag, err := names.ParseControllerTag(consumeDetails.ControllerInfo.ControllerTag)
		if err != nil {
			return errors.Trace(err)
		}
		arg.ControllerInfo = &crossmodel.ControllerInfo{
			ControllerTag: controllerTag,
			Addrs:         consumeDetails.ControllerInfo.Addrs,
			CACert:        consumeDetails.ControllerInfo.CACert,
		}
	}
	_, err = targetClient.Consume(arg)
	return errors.Trace(err)
}

// validateEndpoints determines if all endpoints are valid.
// Each endpoint is either from local service or remote.
// If more than one remote endpoint are supplied, the input argument are considered invalid.
func (c *addRelationCommand) validateEndpoints(all []string) error {
	for _, endpoint := range all {
		if featureflag.Enabled(feature.CrossModelRelations) {
			// We can only determine if this is a remote endpoint with 100%.
			// If we cannot parse it, it may still be a valid local endpoint...
			// so ignoring parsing error,
			if url, err := crossmodel.ParseApplicationURL(endpoint); err == nil {
				if c.remoteEndpoint != nil {
					return errors.NotSupportedf("providing more than one remote endpoints")
				}
				c.remoteEndpoint = url
				c.Endpoints = append(c.Endpoints, url.ApplicationName)
				continue
			}
		}
		// at this stage, we are assuming that this could be a local endpoint
		if err := validateLocalEndpoint(endpoint, ":"); err != nil {
			return err
		}
		c.Endpoints = append(c.Endpoints, endpoint)
	}
	return nil
}

// validateLocalEndpoint determines if given endpoint could be a valid
func validateLocalEndpoint(endpoint string, sep string) error {
	i := strings.Index(endpoint, sep)
	applicationName := endpoint
	if i != -1 {
		// not a valid endpoint as sep either at the start or the end of the name
		if i == 0 || i == len(endpoint)-1 {
			return errors.NotValidf("endpoint %q", endpoint)
		}

		parts := strings.SplitN(endpoint, sep, -1)
		if rightCount := len(parts) == 2; !rightCount {
			// not valid if there are not exactly 2 parts.
			return errors.NotValidf("endpoint %q", endpoint)
		}

		applicationName = parts[0]

		if valid := localEndpointRegEx.MatchString(parts[1]); !valid {
			return errors.NotValidf("endpoint %q", endpoint)
		}
	}

	if valid := names.IsValidApplication(applicationName); !valid {
		return errors.NotValidf("application name %q", applicationName)
	}
	return nil
}
