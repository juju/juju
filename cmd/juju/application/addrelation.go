// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"net"
	"regexp"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/application"
	"github.com/juju/juju/api/applicationoffers"
	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/crossmodel"
)

const addRelationDoc = `
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

For a cross model relation, if the consuming side is behind a firewall and/or NAT is used for outbound traffic,
it is possible to use the --via option to inform the offering side the source of traffic so that any required
firewall ports may be opened.

Examples:
    $ juju add-relation wordpress mysql
        where "wordpress" and "mysql" will be internally expanded to "wordpress:db" and "mysql:server" respectively

    $ juju add-relation wordpress someone/prod.mysql
        where "wordpress" will be internally expanded to "wordpress:db"

    $ juju add-relation wordpress someone/prod.mysql --via 192.168.0.0/16
    
    $ juju add-relation wordpress someone/prod.mysql --via 192.168.0.0/16,10.0.0.0/8

`

var localEndpointRegEx = regexp.MustCompile("^" + names.RelationSnippet + "$")

// NewAddRelationCommand returns a command to add a relation between 2 applications.
func NewAddRelationCommand() cmd.Command {
	return modelcmd.Wrap(&addRelationCommand{})
}

// addRelationCommand adds a relation between two application endpoints.
type addRelationCommand struct {
	modelcmd.ModelCommandBase
	endpoints         []string
	viaCIDRs          []string
	viaValue          string
	remoteEndpoint    *crossmodel.OfferURL
	addRelationAPI    applicationAddRelationAPI
	consumeDetailsAPI applicationConsumeDetailsAPI
}

func (c *addRelationCommand) Info() *cmd.Info {
	addCmd := &cmd.Info{
		Name:    "add-relation",
		Aliases: []string{"relate"},
		Args:    "<application1>[:<endpoint name1>] <application2>[:<endpoint name2>]",
		Purpose: "Add a relation between two application endpoints.",
		Doc:     addRelationDoc,
	}
	return jujucmd.Info(addCmd)
}

func (c *addRelationCommand) Init(args []string) error {
	if len(args) != 2 {
		return errors.Errorf("a relation must involve two applications")
	}
	if err := c.validateEndpoints(args); err != nil {
		return err
	}
	if err := c.validateCIDRs(); err != nil {
		return err
	}
	if c.remoteEndpoint == nil && len(c.viaCIDRs) > 0 {
		return errors.New("the --via option can only be used when relating to offers in a different model")
	}
	return nil
}

func (c *addRelationCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.viaValue, "via", "", "for cross model relations, specify the egress subnets for outbound traffic")
}

// applicationAddRelationAPI defines the API methods that application add relation command uses.
type applicationAddRelationAPI interface {
	Close() error
	BestAPIVersion() int
	AddRelation(endpoints, viaCIDRs []string) (*params.AddRelationResults, error)
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

func (c *addRelationCommand) getOffersAPI(url *crossmodel.OfferURL) (applicationConsumeDetailsAPI, error) {
	if c.consumeDetailsAPI != nil {
		return c.consumeDetailsAPI, nil
	}

	root, err := c.CommandBase.NewAPIRoot(c.ClientStore(), url.Source, "")
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
		if client.BestAPIVersion() < 5 {
			// old client does not have cross-model capability.
			return errors.NotSupportedf("cannot add relation to %s: remote endpoints", c.remoteEndpoint.String())
		}
		if c.remoteEndpoint.Source == "" {
			var err error
			controllerName, err := c.ControllerName()
			if err != nil {
				return errors.Trace(err)
			}
			c.remoteEndpoint.Source = controllerName
		}
		if err := c.maybeConsumeOffer(client); err != nil {
			return errors.Trace(err)
		}
	}

	_, err = client.AddRelation(c.endpoints, c.viaCIDRs)
	if params.IsCodeUnauthorized(err) {
		common.PermissionsMessage(ctx.Stderr, "add a relation")
	}
	if params.IsCodeAlreadyExists(err) {
		// It's not a real error, mention about it, log it and move along
		logger.Infof("%s", err)
		ctx.Infof("%s", err)
		err = nil
	}
	return block.ProcessBlockedError(err, block.BlockChange)
}

func (c *addRelationCommand) maybeConsumeOffer(targetClient applicationAddRelationAPI) error {
	sourceClient, err := c.getOffersAPI(c.remoteEndpoint)
	if err != nil {
		return errors.Trace(err)
	}
	defer sourceClient.Close()

	// Get the details of the remote offer - this will fail with a permission
	// error if the user isn't authorised to consume the offer.
	consumeDetails, err := sourceClient.GetConsumeDetails(c.remoteEndpoint.AsLocal().String())
	if err != nil {
		return errors.Trace(err)
	}
	// Parse the offer details URL and add the source controller so
	// things like status can show the original source of the offer.
	offerURL, err := crossmodel.ParseOfferURL(consumeDetails.Offer.OfferURL)
	if err != nil {
		return errors.Trace(err)
	}
	offerURL.Source = c.remoteEndpoint.Source
	consumeDetails.Offer.OfferURL = offerURL.String()

	// Consume is idempotent so even if the offer has been consumed previously,
	// it's safe to do so again.
	arg := crossmodel.ConsumeApplicationArgs{
		Offer:            *consumeDetails.Offer,
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
			Alias:         offerURL.Source,
			Addrs:         consumeDetails.ControllerInfo.Addrs,
			CACert:        consumeDetails.ControllerInfo.CACert,
		}
	}
	_, err = targetClient.Consume(arg)
	return errors.Trace(err)
}

// validateEndpoints determines if all endpoints are valid.
// Each endpoint is either from local application or remote.
// If more than one remote endpoint are supplied, the input argument are considered invalid.
func (c *addRelationCommand) validateEndpoints(all []string) error {
	for _, endpoint := range all {
		// We can only determine if this is a remote endpoint with 100%.
		// If we cannot parse it, it may still be a valid local endpoint...
		// so ignoring parsing error,
		if url, err := crossmodel.ParseOfferURL(endpoint); err == nil {
			if c.remoteEndpoint != nil {
				return errors.NotSupportedf("providing more than one remote endpoints")
			}
			c.remoteEndpoint = url
			c.endpoints = append(c.endpoints, url.ApplicationName)
			continue
		}
		// at this stage, we are assuming that this could be a local endpoint
		if err := validateLocalEndpoint(endpoint, ":"); err != nil {
			return err
		}
		c.endpoints = append(c.endpoints, endpoint)
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

func (c *addRelationCommand) validateCIDRs() error {
	if c.viaValue == "" {
		return nil
	}
	c.viaCIDRs = strings.Split(
		strings.Replace(c.viaValue, " ", "", -1), ",")
	for _, cidr := range c.viaCIDRs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return err
		}
		if cidr == "0.0.0.0/0" {
			return errors.Errorf("CIDR %q not allowed", cidr)
		}
	}
	return nil
}
