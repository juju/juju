// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v6"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/model"
)

const findCommandDoc = `
Find which offered application endpoints are available to the current user.

This command is aimed for a user who wants to discover what endpoints are available to them.

By default the search is scoped to a single controller: the one named in the 
offer URL, or the current controller if no URL is given.
Use the ` + "`--all-controllers`" + ` flag to search across every controller registered 
locally (see ` + "`juju controllers`" + `).
Results are merged into a single list, each offer namespaced by the controller that hosts it.
`

const findCommandExamples = `
Find offers on the current controller:

    juju find-offers
    juju find-offers staging/mymodel
    juju find-offers --interface mysql
    juju find-offers --url staging/mymodel.db2
    juju find-offers --offer db2

Find offers on a named controller:

    juju find-offers mycontroller:

Find offers across every locally registered controller:

    juju find-offers --all-controllers
    juju find-offers -a --interface mysql
`

type findCommand struct {
	RemoteEndpointsCommandBase

	url            string
	source         string
	modelQualifier string
	modelName      string
	offerName      string
	interfaceName  string
	allControllers bool

	out        cmd.Output
	newAPIFunc func(context.Context, string) (FindAPI, error)
}

// NewFindEndpointsCommand constructs command that
// allows to find offered application endpoints.
func NewFindEndpointsCommand() cmd.Command {
	findCmd := &findCommand{}
	findCmd.newAPIFunc = func(ctx context.Context, controllerName string) (FindAPI, error) {
		return findCmd.NewRemoteEndpointsAPI(ctx, controllerName)
	}
	return modelcmd.Wrap(findCmd)
}

// Init implements Command.Init.
func (c *findCommand) Init(args []string) (err error) {
	if c.offerName != "" && c.url != "" {
		return errors.New("cannot specify both a URL term and offer term")
	}
	url, err := cmd.ZeroOrOneArgs(args)
	if err != nil {
		return errors.Trace(err)
	}
	if url != "" {
		if c.url != "" {
			return errors.New("URL term cannot be specified twice")
		}
		c.url = url
	}
	return nil
}

// controllerSource returns the controller name to search for offers on, or the
// empty string when the search should fan out across all registered
// controllers.
func (c *findCommand) controllerSource() string {
	if c.allControllers {
		return ""
	}
	return c.source
}

// Info implements Command.Info.
func (c *findCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "find-offers",
		Purpose:  "Find offered application endpoints.",
		Doc:      findCommandDoc,
		Examples: findCommandExamples,
		SeeAlso: []string{
			"show-offer",
		},
	})
}

// SetFlags implements Command.SetFlags.
func (c *findCommand) SetFlags(f *gnuflag.FlagSet) {
	c.RemoteEndpointsCommandBase.SetFlags(f)
	f.StringVar(&c.url, "url", "", "Return results matching the offer URL")
	f.StringVar(&c.interfaceName, "interface", "", "Return results matching the interface name")
	f.StringVar(&c.offerName, "offer", "", "Return results matching the offer name")
	f.BoolVar(&c.allControllers, "all-controllers", false, "Search for offers across all registered controllers")
	f.BoolVar(&c.allControllers, "a", false, "")
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatFindTabular,
	})
}

// Run implements Command.Run.
func (c *findCommand) Run(ctx *cmd.Context) (err error) {
	if err := c.validateOrSetURL(); err != nil {
		return errors.Trace(err)
	}
	accountDetails, err := c.CurrentAccountDetails()
	if err != nil {
		return err
	}
	loggedInUser := names.NewUserTag(accountDetails.User)

	filter := crossmodel.ApplicationOfferFilter{
		ModelQualifier: model.Qualifier(c.modelQualifier),
		ModelName:      c.modelName,
		OfferName:      c.offerName,
	}
	if c.interfaceName != "" {
		filter.Endpoints = []crossmodel.EndpointFilterTerm{{
			Interface: c.interfaceName,
		}}
	}

	var output map[string]ApplicationOfferResult
	if c.allControllers {
		output, err = c.findAcrossControllers(ctx, loggedInUser, filter)
	} else {
		output, err = c.findOnController(ctx, c.source, loggedInUser, filter)
	}
	if err != nil {
		return errors.Trace(err)
	}

	if len(output) == 0 {
		return errors.New("no matching application offers found")
	}
	return c.out.Write(ctx, output)
}

// findOnController queries a single controller for matching offers.
func (c *findCommand) findOnController(
	ctx context.Context, controllerName string, loggedInUser names.UserTag, filter crossmodel.ApplicationOfferFilter,
) (map[string]ApplicationOfferResult, error) {
	api, err := c.newAPIFunc(ctx, controllerName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer api.Close()

	found, err := api.FindApplicationOffers(ctx, filter)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return convertFoundOffers(controllerName, loggedInUser, found...)
}

// findAcrossControllers fans out the offer search across every registered
// controller in the client store, querying each concurrently and merging the
// results. Each result's offer URL is namespaced by its source controller, so
// the merged output presents a unified, cross-controller offer catalogue.
//
// A single unreachable or erroring controller does not fail the whole search:
// the error is reported as a warning and the remaining controllers' offers are
// still returned. The command only fails if no offers are found at all.
func (c *findCommand) findAcrossControllers(
	ctx *cmd.Context, loggedInUser names.UserTag, filter crossmodel.ApplicationOfferFilter,
) (map[string]ApplicationOfferResult, error) {
	controllers, err := c.ClientStore().AllControllers()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(controllers) == 0 {
		return nil, errors.New("no controllers registered")
	}

	// Query controllers in a deterministic order so that merge behaviour and
	// per-controller warnings are stable.
	controllerNames := make([]string, 0, len(controllers))
	for name := range controllers {
		controllerNames = append(controllerNames, name)
	}
	sort.Strings(controllerNames)

	type result struct {
		controllerName string
		offers         map[string]ApplicationOfferResult
		err            error
	}
	results := make([]result, len(controllerNames))
	var wg sync.WaitGroup
	for i, controllerName := range controllerNames {
		wg.Add(1)
		go func(i int, controllerName string) {
			defer wg.Done()
			offers, err := c.findOnController(ctx, controllerName, loggedInUser, filter)
			results[i] = result{controllerName: controllerName, offers: offers, err: err}
		}(i, controllerName)
	}
	wg.Wait()

	merged := make(map[string]ApplicationOfferResult)
	for _, r := range results {
		if r.err != nil {
			fmt.Fprintf(ctx.GetStderr(), "could not search controller %q: %v\n", r.controllerName, r.err)
			continue
		}
		for url, offer := range r.offers {
			merged[url] = offer
		}
	}
	return merged, nil
}

func (c *findCommand) validateOrSetURL() error {
	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}
	if c.url == "" {
		// When fanning out across all controllers there is no single source
		// controller; leave c.source empty and do not pin the URL to one
		// controller.
		if !c.allControllers {
			c.url = controllerName + ":"
			c.source = controllerName
		}
		return nil
	}
	urlParts, err := crossmodel.ParseOfferURLParts(c.url)
	if err != nil {
		return errors.Trace(err)
	}
	if urlParts.Source != "" {
		if c.allControllers {
			return errors.New("cannot specify a controller in the URL with --all-controllers")
		}
		c.source = urlParts.Source
	} else if !c.allControllers {
		c.source = controllerName
	}
	qualifier := urlParts.ModelQualifier
	if qualifier == "" {
		accountDetails, err := c.CurrentAccountDetails()
		if err != nil {
			return errors.Trace(err)
		}
		qualifier = accountDetails.User
	}
	c.modelQualifier = qualifier
	c.modelName = urlParts.ModelName
	c.offerName = urlParts.Name
	return nil
}

// FindAPI defines the API methods that cross model find command uses.
type FindAPI interface {
	Close() error
	FindApplicationOffers(ctx context.Context, filters ...crossmodel.ApplicationOfferFilter) ([]*crossmodel.ApplicationOfferDetails, error)
}

// ApplicationOfferResult defines the serialization behaviour of an application offer.
// This is used in map-style yaml output where offer URL is the key.
type ApplicationOfferResult struct {
	// Access is the level of access the user has on the offer.
	Access string `yaml:"access" json:"access"`

	// Endpoints is the list of offered application endpoints.
	Endpoints map[string]RemoteEndpoint `yaml:"endpoints" json:"endpoints"`

	// Users are the users who can access the offer.
	Users map[string]OfferUser `yaml:"users,omitempty" json:"users,omitempty"`
}

func accessForUser(user names.UserTag, users []crossmodel.OfferUserDetails) string {
	for _, u := range users {
		if u.UserName == user.Id() {
			return string(u.Access)
		}
	}
	return "-"
}

// convertFoundOffers takes any number of api-formatted remote applications and
// creates a collection of ui-formatted applications.
func convertFoundOffers(
	store string, loggedInUser names.UserTag, offers ...*crossmodel.ApplicationOfferDetails,
) (map[string]ApplicationOfferResult, error) {
	if len(offers) == 0 {
		return nil, nil
	}
	output := make(map[string]ApplicationOfferResult, len(offers))
	for _, one := range offers {
		access := accessForUser(loggedInUser, one.Users)
		app := ApplicationOfferResult{
			Access:    access,
			Endpoints: convertRemoteEndpoints(one.Endpoints...),
			Users:     convertUsers(one.Users...),
		}
		url, err := crossmodel.ParseOfferURL(one.OfferURL)
		if err != nil {
			return nil, err
		}
		if url.Source == "" {
			url.Source = store
		}
		output[url.String()] = app
	}
	return output, nil
}
