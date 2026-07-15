// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/client/application"
	"github.com/juju/juju/api/client/applicationoffers"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/rpc/params"
)

var usageConsumeSummary = `
Add a remote offer to the model.`[1:]

var usageConsumeDetails = `
Adds a remote offer to the model. Relations can be created later using ` + "`juju integrate`" + `.

The path to the remote offer is formatted as follows:

    [<controller name>:][<model qualifier>/]<model name>.<application name>

If the model qualifier is omitted, Juju will use the user that is currently
logged in to the controller providing the offer.

If the controller name is omitted, Juju looks for the offer on the currently
active controller. Pass ` + "`--all-controllers`" + ` to also search the other
controllers registered locally (see ` + "`juju controllers`" + `) if the offer is
not found on the current controller; the offering controller must be registered
locally for its name to resolve. To target a specific controller directly,
include the controller name in the offer path.
`[1:]

const usageConsumeExamples = `
    juju consume othermodel.mysql
    juju consume prod/othermodel.mysql
    juju consume anothercontroller:prod/othermodel.mysql
    juju consume --all-controllers othermodel.mysql
`

// NewConsumeCommand returns a command to add remote offers to
// the model.
func NewConsumeCommand() cmd.Command {
	return modelcmd.Wrap(&consumeCommand{})
}

// consumeCommand adds remote offers to the model without
// relating them to other applications.
type consumeCommand struct {
	modelcmd.ModelCommandBase
	sourceAPI         applicationConsumeDetailsAPI
	targetAPI         applicationConsumeAPI
	remoteApplication string
	applicationAlias  string
	allControllers    bool

	// newSourceAPIForController, when set (in tests), returns a source offers
	// client for a named controller, allowing the per-controller resolution
	// fan-out to be exercised. Production code leaves this nil and dials via
	// the client store.
	newSourceAPIForController func(controllerName string) (applicationConsumeDetailsAPI, error)
}

// notFoundOfferError is returned by GetConsumeDetails when a controller does
// not host the requested offer. When resolving an unqualified offer URL, this
// signals that the search should continue with the next controller.
func isOfferNotFound(err error) bool {
	return errors.Is(err, errors.NotFound) || params.IsCodeNotFound(err)
}

// Info implements cmd.Command.
func (c *consumeCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "consume",
		Args:     "<remote offer path> [<local application name>]",
		Purpose:  usageConsumeSummary,
		Doc:      usageConsumeDetails,
		Examples: usageConsumeExamples,
		SeeAlso: []string{
			"integrate",
			"offer",
			"remove-saas",
		},
	})
}

// SetFlags implements cmd.Command.
func (c *consumeCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.allControllers, "all-controllers", false,
		"Also search other registered controllers when the offer's controller is not named and the offer is not on the current controller")
}

// Init implements cmd.Command.
func (c *consumeCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("no remote offer specified")
	}
	c.remoteApplication = args[0]
	if len(args) > 1 {
		if !names.IsValidApplication(args[1]) {
			return errors.Errorf("invalid application name %q", args[1])
		}
		c.applicationAlias = args[1]
		return cmd.CheckEmpty(args[2:])
	}
	return nil
}

func (c *consumeCommand) getTargetAPI(ctx context.Context) (applicationConsumeAPI, error) {
	if c.targetAPI != nil {
		return c.targetAPI, nil
	}
	root, err := c.NewAPIRoot(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return application.NewClient(root), nil
}

// getSourceAPI returns an offers client for the named controller. If a single
// source API was injected (in tests) it is returned regardless of the
// controller name; if a per-controller factory was injected it is used;
// otherwise a real client is dialled via the client store.
func (c *consumeCommand) getSourceAPI(ctx context.Context, controllerName string) (applicationConsumeDetailsAPI, error) {
	if c.sourceAPI != nil {
		return c.sourceAPI, nil
	}
	if c.newSourceAPIForController != nil {
		return c.newSourceAPIForController(controllerName)
	}
	root, err := c.CommandBase.NewAPIRoot(ctx, c.ClientStore(), controllerName, "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	return applicationoffers.NewClient(root), nil
}

// resolveConsumeDetails fetches the consume details for the offer at url. When
// url.Source is set, it queries exactly that controller. When url.Source is
// empty, it searches the currently active controller first and then the other
// registered controllers, returning the details from the first controller that
// hosts the offer and recording that controller as the resolved source.
//
// On success it returns the still-open source client; the caller is responsible
// for closing it. Any clients opened for controllers that did not host the
// offer are closed before returning.
func (c *consumeCommand) resolveConsumeDetails(ctx *cmd.Context, url crossmodel.OfferURL) (applicationConsumeDetailsAPI, params.ConsumeOfferDetails, string, error) {
	localURL := url.AsLocal().String()

	// Explicit source controller: query just that one.
	if url.Source != "" {
		client, err := c.getSourceAPI(ctx, url.Source)
		if err != nil {
			return nil, params.ConsumeOfferDetails{}, "", errors.Trace(err)
		}
		details, err := client.GetConsumeDetails(ctx, localURL)
		if err != nil {
			_ = client.Close()
			return nil, params.ConsumeOfferDetails{}, "", errors.Trace(err)
		}
		return client, details, url.Source, nil
	}

	// No source controller: search the current controller first, then the
	// rest of the registered controllers.
	candidates, err := c.candidateControllers()
	if err != nil {
		return nil, params.ConsumeOfferDetails{}, "", errors.Trace(err)
	}
	// Only emit per-controller warnings when there is an actual fan-out across
	// multiple controllers; with a single controller the returned error is
	// enough and a warning would just be noise.
	fanningOut := len(candidates) > 1
	// Track the last non-"not found" error so that, if no controller hosts
	// the offer, we surface a real failure (auth/connection) rather than a
	// misleading "not found".
	var lastErr error
	for _, controllerName := range candidates {
		client, err := c.getSourceAPI(ctx, controllerName)
		if err != nil {
			// A controller we cannot dial is not fatal while searching;
			// remember it and try the next one.
			if fanningOut {
				ctx.Warningf("could not reach controller %q: %v", controllerName, err)
			}
			lastErr = err
			continue
		}
		details, err := client.GetConsumeDetails(ctx, localURL)
		if err == nil {
			return client, details, controllerName, nil
		}
		_ = client.Close()
		if isOfferNotFound(err) {
			// Offer not on this controller; keep looking.
			continue
		}
		// A real error (auth, connection) is worth surfacing but should not
		// stop the search across other controllers.
		if fanningOut {
			ctx.Warningf("could not search controller %q: %v", controllerName, err)
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, params.ConsumeOfferDetails{}, "", errors.Trace(lastErr)
	}
	return nil, params.ConsumeOfferDetails{}, "", errors.NotFoundf("offer %q on any registered controller", localURL)
}

// candidateControllers returns the controllers to search for an unqualified
// offer. When --all-controllers is not set (or a source API has been injected
// in tests), only the current controller is returned. When --all-controllers is
// set, the current controller is first and the remaining registered controllers
// follow in deterministic order.
func (c *consumeCommand) candidateControllers() ([]string, error) {
	current, err := c.ControllerName()
	if err != nil {
		return nil, errors.Trace(err)
	}
	// A test may have injected a source API — treat it as current-only
	// regardless of the flag so existing single-controller tests are unaffected.
	if c.sourceAPI != nil || !c.allControllers {
		return []string{current}, nil
	}

	all, err := c.ClientStore().AllControllers()
	if err != nil {
		return nil, errors.Trace(err)
	}

	rest := make([]string, 0, len(all))
	for name := range all {
		if name != current {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	return append([]string{current}, rest...), nil
}

// Run adds the requested remote offer to the model. Implements
// cmd.Command.
func (c *consumeCommand) Run(ctx *cmd.Context) error {
	accountDetails, err := c.CurrentAccountDetails()
	if err != nil {
		return errors.Trace(err)
	}
	url, err := crossmodel.ParseOfferURL(c.remoteApplication)
	if err != nil {
		return errors.Trace(err)
	}
	if url.HasEndpoint() {
		return errors.Errorf("saas offer %q shouldn't include endpoint", c.remoteApplication)
	}
	if url.ModelQualifier == "" {
		url.ModelQualifier = accountDetails.User
		c.remoteApplication = url.Path()
	}
	// Fetch the consume details, resolving which controller hosts the offer
	// when the offer URL does not name one explicitly.
	sourceClient, consumeDetails, resolvedSource, err := c.resolveConsumeDetails(ctx, url)
	if err != nil {
		return errors.Trace(err)
	}
	defer sourceClient.Close()

	// Parse the offer details URL and add the (possibly resolved) source
	// controller so things like status can show the original source of the
	// offer.
	offerURL, err := crossmodel.ParseOfferURL(consumeDetails.Offer.OfferURL)
	if err != nil {
		return errors.Trace(err)
	}
	offerURL.Source = resolvedSource
	consumeDetails.Offer.OfferURL = offerURL.String()

	targetClient, err := c.getTargetAPI(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	defer targetClient.Close()

	arg := crossmodel.ConsumeApplicationArgs{
		Offer:            *consumeDetails.Offer,
		ApplicationAlias: c.applicationAlias,
		Macaroon:         consumeDetails.Macaroon,
	}
	if consumeDetails.ControllerInfo != nil {
		controllerTag, err := names.ParseControllerTag(consumeDetails.ControllerInfo.ControllerTag)
		if err != nil {
			return errors.Trace(err)
		}
		arg.ControllerInfo = &crossmodel.ControllerInfo{
			ControllerUUID: controllerTag.Id(),
			Alias:          consumeDetails.ControllerInfo.Alias,
			Addrs:          consumeDetails.ControllerInfo.Addrs,
			CACert:         consumeDetails.ControllerInfo.CACert,
		}
	}
	localName, err := targetClient.Consume(ctx, arg)
	if err != nil {
		return block.ProcessBlockedError(errors.Annotatef(err, "could not consume %v", url.AsLocal().String()), block.BlockChange)
	}
	ctx.Infof("Added %s as %s", c.remoteApplication, localName)
	return nil
}

type applicationConsumeAPI interface {
	Close() error
	Consume(context.Context, crossmodel.ConsumeApplicationArgs) (string, error)
}

type applicationConsumeDetailsAPI interface {
	Close() error
	GetConsumeDetails(context.Context, string) (params.ConsumeOfferDetails, error)
}
