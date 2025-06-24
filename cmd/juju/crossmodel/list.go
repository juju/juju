// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/client/applicationoffers"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/crossmodel"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/jujuclient"
)

const listCommandDoc = `
List information about applications' endpoints that have been shared and who is connected.

The default tabular output shows each user connected (relating to) the offer, and the 
relation id of the relation.

The summary output shows one row per offer, with a count of active/total relations.

The YAML output shows additional information about the source of connections, including
the source model UUID.

The output can be filtered by:
 - interface: the interface name of the endpoint
 - application: the name of the offered application
 - connected user: the name of a user who has a relation to the offer
 - allowed consumer: the name of a user allowed to consume the offer
 - active only: only show offers which are in use (are related to)

`

const listCommandExamples = `
    juju offers
    juju offers -m model
    juju offers --interface db2
    juju offers --application mysql
    juju offers --connected-user fred
    juju offers --allowed-consumer mary
    juju offers hosted-mysql
    juju offers hosted-mysql --active-only
`

// listCommand returns storage instances.
type listCommand struct {
	modelcmd.ModelCommandBase

	out cmd.Output

	newAPIFunc    func(ctx context.Context) (ListAPI, error)
	refreshModels func(context.Context, jujuclient.ClientStore, string) error

	activeOnly        bool
	interfaceName     string
	applicationName   string
	connectedUserName string
	consumerName      string
	offerName         string
	filters           []crossmodel.ApplicationOfferFilter
}

// NewListEndpointsCommand constructs new list endpoint command.
func NewListEndpointsCommand() cmd.Command {
	listCmd := &listCommand{}
	listCmd.newAPIFunc = func(ctx context.Context) (ListAPI, error) {
		return listCmd.NewApplicationOffersAPI(ctx)
	}
	listCmd.refreshModels = listCmd.ModelCommandBase.RefreshModels
	return modelcmd.Wrap(listCmd)
}

// NewApplicationOffersAPI returns an application offers api for the root api endpoint
// that the command returns.
func (c *listCommand) NewApplicationOffersAPI(ctx context.Context) (*applicationoffers.Client, error) {
	root, err := c.NewControllerAPIRoot(ctx)
	if err != nil {
		return nil, err
	}
	return applicationoffers.NewClient(root), nil
}

// Init implements Command.Init.
func (c *listCommand) Init(args []string) (err error) {
	offerName, err := cmd.ZeroOrOneArgs(args)
	if err != nil {
		return errors.Trace(err)
	}
	c.offerName = offerName
	return nil
}

// Info implements Command.Info.
func (c *listCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "offers",
		Args:     "[<offer-name>]",
		Aliases:  []string{"list-offers"},
		Purpose:  "Lists shared endpoints.",
		Doc:      listCommandDoc,
		Examples: listCommandExamples,
		SeeAlso: []string{
			"find-offers",
			"show-offer",
		},
	})
}

// SetFlags implements Command.SetFlags.
func (c *listCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.StringVar(&c.applicationName, "application", "", "return results matching the application")
	f.StringVar(&c.interfaceName, "interface", "", "return results matching the interface name")
	f.StringVar(&c.consumerName, "allowed-consumer", "", "return results where the user is allowed to consume the offer")
	f.StringVar(&c.connectedUserName, "connected-user", "", "return results where the user has a connection to the offer")
	f.BoolVar(&c.activeOnly, "active-only", false, "only return results where the offer is in use")
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatListTabular,
		"summary": formatListSummary,
	})
}

// Run implements Command.Run.
func (c *listCommand) Run(ctx *cmd.Context) (err error) {
	api, err := c.newAPIFunc(ctx)
	if err != nil {
		return err
	}
	defer api.Close()

	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}

	modelName, _, err := c.ModelDetails(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if !jujuclient.IsQualifiedModelName(modelName) {
		store := modelcmd.QualifyingClientStore{c.ClientStore()}
		var err error
		modelName, err = store.QualifiedModelName(controllerName, modelName)
		if err != nil {
			return errors.Trace(err)
		}
	}

	unqualifiedModelName, qualifier, err := jujuclient.SplitFullyQualifiedModelName(modelName)
	if err != nil {
		return errors.Trace(err)
	}
	c.filters = []crossmodel.ApplicationOfferFilter{{
		ModelQualifier:  coremodel.Qualifier(qualifier),
		ModelName:       unqualifiedModelName,
		ApplicationName: c.applicationName,
	}}
	if c.offerName != "" {
		c.filters[0].OfferName = fmt.Sprintf("^%v$", regexp.QuoteMeta(c.offerName))
	}
	if c.interfaceName != "" {
		c.filters[0].Endpoints = []crossmodel.EndpointFilterTerm{{
			Interface: c.interfaceName,
		}}
	}
	if c.connectedUserName != "" {
		c.filters[0].ConnectedUsers = []string{c.connectedUserName}
	}
	if c.consumerName != "" {
		c.filters[0].AllowedConsumers = []string{c.consumerName}
	}

	offeredApplications, err := api.ListOffers(ctx, c.filters...)
	if err != nil {
		return err
	}

	// For now, all offers come from the one controller.
	data, err := formatApplicationOfferDetails(controllerName, offeredApplications, c.activeOnly)
	if err != nil {
		return errors.Annotate(err, "failed to format found applications")
	}

	return c.out.Write(ctx, data)
}

// ListAPI defines the API methods that list endpoints command use.
type ListAPI interface {
	Close() error
	ListOffers(ctx context.Context, filters ...crossmodel.ApplicationOfferFilter) ([]*crossmodel.ApplicationOfferDetails, error)
}

// ListOfferItem defines the serialization behaviour of an offer item in endpoints list.
type ListOfferItem struct {
	// OfferName is the name of the offer.
	OfferName string `yaml:"-" json:"-"`

	// ApplicationName is the application backing this offer.
	ApplicationName string `yaml:"application" json:"application"`

	// Store is the controller hosting this offer.
	Source string `yaml:"store,omitempty" json:"store,omitempty"`

	// CharmURL is the charm URL of this application.
	CharmURL string `yaml:"charm,omitempty" json:"charm,omitempty"`

	// OfferURL is part of Juju location where this offer is shared relative to the store.
	OfferURL string `yaml:"offer-url" json:"offer-url"`

	// Endpoints is a list of application endpoints.
	Endpoints map[string]RemoteEndpoint `yaml:"endpoints" json:"endpoints"`

	// Connections holds details of connections to the offer.
	Connections []offerConnectionDetails `yaml:"connections,omitempty" json:"connections,omitempty"`

	// Users are the users who can consume the offer.
	Users map[string]OfferUser `yaml:"users,omitempty" json:"users,omitempty"`
}

type offeredApplications map[string]ListOfferItem

type offerConnectionStatus struct {
	Current string `json:"current" yaml:"current"`
	Message string `json:"message,omitempty" yaml:"message,omitempty"`
	Since   string `json:"since,omitempty" yaml:"since,omitempty"`
}

type offerConnectionDetails struct {
	SourceModelUUID string                `json:"source-model-uuid" yaml:"source-model-uuid"`
	Username        string                `json:"username" yaml:"username"`
	RelationId      int                   `json:"relation-id" yaml:"relation-id"`
	Endpoint        string                `json:"endpoint" yaml:"endpoint"`
	Status          offerConnectionStatus `json:"status" yaml:"status"`
	IngressSubnets  []string              `json:"ingress-subnets,omitempty" yaml:"ingress-subnets,omitempty"`
}

func formatApplicationOfferDetails(store string, all []*crossmodel.ApplicationOfferDetails, activeOnly bool) (offeredApplications, error) {
	result := make(offeredApplications)
	for _, one := range all {
		if activeOnly && len(one.Connections) == 0 {
			continue
		}
		url, err := crossmodel.ParseOfferURL(one.OfferURL)
		if err != nil {
			return nil, errors.Annotatef(err, "%v", one.OfferURL)
		}
		if url.Source == "" {
			url.Source = store
		}

		// Store offers by name.
		result[one.OfferName] = convertOfferToListItem(url, one)
	}
	return result, nil
}

func convertOfferToListItem(url *crossmodel.OfferURL, offer *crossmodel.ApplicationOfferDetails) ListOfferItem {
	item := ListOfferItem{
		OfferName:       offer.OfferName,
		ApplicationName: offer.ApplicationName,
		Source:          url.Source,
		CharmURL:        offer.CharmURL,
		OfferURL:        offer.OfferURL,
		Endpoints:       convertCharmEndpoints(offer.Endpoints...),
		Users:           convertUsers(offer.Users...),
	}
	for _, conn := range offer.Connections {
		item.Connections = append(item.Connections, offerConnectionDetails{
			SourceModelUUID: conn.SourceModelUUID,
			Username:        conn.Username,
			RelationId:      conn.RelationId,
			Endpoint:        conn.Endpoint,
			Status: offerConnectionStatus{
				Current: conn.Status.String(),
				Message: conn.Message,
				Since:   friendlyDuration(conn.Since),
			},
			IngressSubnets: conn.IngressSubnets,
		})
	}
	return item
}

func friendlyDuration(when *time.Time) string {
	if when == nil {
		return ""
	}
	return common.UserFriendlyDuration(*when, time.Now())
}

// convertCharmEndpoints takes any number of charm relations and
// creates a collection of ui-formatted endpoints.
func convertCharmEndpoints(relations ...charm.Relation) map[string]RemoteEndpoint {
	if len(relations) == 0 {
		return nil
	}
	output := make(map[string]RemoteEndpoint, len(relations))
	for _, one := range relations {
		output[one.Name] = RemoteEndpoint{one.Name, one.Interface, string(one.Role)}
	}
	return output
}
