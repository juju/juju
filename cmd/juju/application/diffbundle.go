// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/juju/bundlechanges/v3"
	"github.com/juju/charm/v8"
	"github.com/juju/charmrepo/v6"
	csparams "github.com/juju/charmrepo/v6/csclient/params"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/api/annotations"
	"github.com/juju/juju/api/application"
	"github.com/juju/juju/api/base"
	apicharms "github.com/juju/juju/api/charms"
	commoncharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/api/modelconfig"
	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	appbundle "github.com/juju/juju/cmd/juju/application/bundle"
	"github.com/juju/juju/cmd/juju/application/store"
	"github.com/juju/juju/cmd/juju/application/utils"
	"github.com/juju/juju/cmd/modelcmd"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
)

const bundleDiffDoc = `
Bundle can be a local bundle file or the name of a bundle in
the charm store. The bundle can also be combined with overlays (in the
same way as the deploy command) before comparing with the model.

The map-machines option works similarly as for the deploy command, but
existing is always assumed, so it doesn't need to be specified.

Config values for comparison are always source from the "current" model
generation.

Examples:
    juju diff-bundle localbundle.yaml
    juju diff-bundle cs:canonical-kubernetes
    juju diff-bundle -m othermodel hadoop-spark
    juju diff-bundle cs:mongodb-cluster --channel beta
    juju diff-bundle cs:canonical-kubernetes --overlay local-config.yaml --overlay extra.yaml
    juju diff-bundle localbundle.yaml --map-machines 3=4

See also:
    deploy
`

// NewDiffBundleCommand returns a command to compare a bundle against
// the selected model.
func NewDiffBundleCommand() cmd.Command {
	cmd := &diffBundleCommand{}
	cmd.charmAdaptorFn = cmd.charmAdaptor
	cmd.newAPIRootFn = func() (base.APICallCloser, error) {
		return cmd.NewAPIRoot()
	}
	cmd.newControllerAPIRootFn = func() (base.APICallCloser, error) {
		return cmd.NewControllerAPIRoot()
	}
	return modelcmd.Wrap(cmd)
}

// diffBundleCommand compares a bundle to a model.
type diffBundleCommand struct {
	modelcmd.ModelCommandBase
	bundle         string
	bundleOverlays []string
	channelStr     string
	channel        corecharm.Channel
	annotations    bool

	bundleMachines map[string]string
	machineMap     string

	charmAdaptorFn         func(base.APICallCloser, *charm.URL) (BundleResolver, error)
	newAPIRootFn           func() (base.APICallCloser, error)
	newControllerAPIRootFn func() (base.APICallCloser, error)
}

// IsSuperCommand is part of cmd.Command.
func (c *diffBundleCommand) IsSuperCommand() bool { return false }

// AllowInterspersedFlags is part of cmd.Command.
func (c *diffBundleCommand) AllowInterspersedFlags() bool { return true }

// Info is part of cmd.Command.
func (c *diffBundleCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "diff-bundle",
		Args:    "<bundle file or name>",
		Purpose: "Compare a bundle with a model and report any differences.",
		Doc:     bundleDiffDoc,
	})
}

// SetFlags is part of cmd.Command.
func (c *diffBundleCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.StringVar(&c.channelStr, "channel", "", "Channel to use when getting the bundle from the charm hub or charm store")
	f.Var(cmd.NewAppendStringsValue(&c.bundleOverlays), "overlay", "Bundles to overlay on the primary bundle, applied in order")
	f.StringVar(&c.machineMap, "map-machines", "", "Indicates how existing machines correspond to bundle machines")
	f.BoolVar(&c.annotations, "annotations", false, "Include differences in annotations")
}

// Init is part of cmd.Command.
func (c *diffBundleCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("no bundle specified")
	}
	c.bundle = args[0]
	// UseExisting is assumed for diffing.
	_, mapping, err := parseMachineMap(c.machineMap)
	if err != nil {
		return errors.Annotate(err, "error in --map-machines")
	}
	c.bundleMachines = mapping
	if c.channelStr != "" {
		c.channel, err = corecharm.ParseChannelNormalize(c.channelStr)
		if err != nil {
			return errors.Annotate(err, "error in --channel")
		}
	}
	return cmd.CheckEmpty(args[1:])
}

// Run is part of cmd.Command.
func (c *diffBundleCommand) Run(ctx *cmd.Context) error {
	apiRoot, err := c.newAPIRootFn()
	if err != nil {
		return errors.Trace(err)
	}
	defer func() { _ = apiRoot.Close() }()

	// Load up the bundle data, with includes and overlays.
	baseSrc, err := c.bundleDataSource(ctx, apiRoot)
	if err != nil {
		return errors.Trace(err)
	}

	bundle, err := appbundle.ComposeAndVerifyBundle(baseSrc, c.bundleOverlays)
	if err != nil {
		return errors.Trace(err)
	}

	if err = c.warnForMissingRelationEndpoints(ctx, bundle); err != nil {
		return errors.Trace(err)
	}

	// Extract the information from the current model.
	model, err := c.readModel(apiRoot)
	if err != nil {
		return errors.Trace(err)
	}
	// Get the differences between them.
	diff, err := bundlechanges.BuildDiff(bundlechanges.DiffConfig{
		Bundle:             bundle,
		Model:              model,
		Logger:             logger,
		IncludeAnnotations: c.annotations,
	})

	if err != nil {
		return errors.Trace(err)
	}

	encoder := yaml.NewEncoder(ctx.Stdout)
	defer func() { _ = encoder.Close() }()
	err = encoder.Encode(diff)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (c *diffBundleCommand) warnForMissingRelationEndpoints(ctx *cmd.Context, bundle *charm.BundleData) error {
	var missing []string
	for _, relPair := range bundle.Relations {
		if len(relPair) != 2 {
			return errors.Errorf("malformed relation %v", relPair)
		}

		if missingRelationEndpoint(relPair[0]) || missingRelationEndpoint(relPair[1]) {
			missing = append(missing, fmt.Sprintf("[%s, %s]", relPair[0], relPair[1]))
		}
	}

	if len(missing) == 0 {
		return nil
	}

	logger.Warningf(
		"The provided bundle includes relations without explicit endpoints, which may appear as extra entries in the diff output.\nTo avoid this in the future, update the endpoints for the following bundle relations:\n - %s",
		strings.Join(missing, "\n - "),
	)

	// Add an extra blank line to separate the diff output from the warning
	// and avoid confusion.
	_, _ = fmt.Fprintln(ctx.Stderr)

	return nil
}

func missingRelationEndpoint(rel string) bool {
	tokens := strings.SplitN(rel, ":", 2)
	return len(tokens) != 2 || tokens[1] == ""
}

func (c *diffBundleCommand) bundleDataSource(ctx *cmd.Context, apiRoot base.APICallCloser) (charm.BundleDataSource, error) {
	ds, err := charm.LocalBundleDataSource(c.bundle)

	// NotValid/NotFound means we should try interpreting it as a charm store
	// bundle URL.
	if err != nil && !errors.IsNotValid(err) && !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	if ds != nil {
		return ds, nil
	}

	// Not a local bundle, so it must be from the charmstore.
	bURL, err := charm.ParseURL(c.bundle)
	if err != nil {
		return nil, errors.Trace(err)
	}
	origin, err := utils.DeduceOrigin(bURL, c.channel)
	if err != nil {
		return nil, errors.Trace(err)
	}
	charmAdaptor, err := c.charmAdaptorFn(apiRoot, bURL)
	if err != nil {
		return nil, errors.Trace(err)
	}
	bundleURL, _, err := charmAdaptor.ResolveBundleURL(bURL, origin)
	if err != nil {
		// Handle the charmhub failures during a resolving a bundle.
		isCharmHub := charm.CharmHub.Matches(bURL.Schema)
		if isCharmHub && errors.IsNotSupported(err) {
			msg := `
Current controller version is not compatible with CharmHub bundles.
Consider using a CharmStore bundle instead.`
			return nil, errors.Errorf(msg[1:])
		} else if isCharmHub && errors.IsNotValid(err) {
			return nil, errors.Errorf("%q can not be found or is not a valid bundle", c.bundle)
		}
		return nil, errors.Trace(err)
	}
	if bundleURL == nil {
		// This isn't a charmstore bundle either! Complain.
		return nil, errors.Errorf("couldn't interpret %q as a local or charmstore bundle", c.bundle)
	}

	// GetBundle creates the directory so we actually want to create a temp
	// directory then add a namespace (bundle name) so that charmhub get
	// bundle can create it.
	dir, err := ioutil.TempDir("", "bundle-diff-")
	if err != nil {
		return nil, errors.Trace(err)
	}
	bundlePath := filepath.Join(dir, bundleURL.Name)
	bundle, err := charmAdaptor.GetBundle(bundleURL, bundlePath)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return store.NewResolvedBundle(bundle), nil
}

func (c *diffBundleCommand) charmAdaptor(apiRoot base.APICallCloser, curl *charm.URL) (BundleResolver, error) {
	charmstoreFallback := func() (store.CharmrepoForDeploy, error) {
		controllerAPIRoot, err := c.newControllerAPIRootFn()
		if err != nil {
			return nil, errors.Trace(err)
		}
		defer func() { _ = controllerAPIRoot.Close() }()
		csURL, err := getCharmStoreAPIURL(controllerAPIRoot)
		if err != nil {
			return nil, errors.Trace(err)
		}

		bakeryClient, err := c.BakeryClient()
		if err != nil {
			return nil, errors.Trace(err)
		}

		risk := csparams.Channel(c.channel.Risk)
		cstoreClient := store.NewCharmStoreClient(bakeryClient, csURL).WithChannel(risk)
		return charmrepo.NewCharmStoreFromClient(cstoreClient), nil
	}

	return store.NewCharmAdaptor(apicharms.NewClient(apiRoot), charmstoreFallback), nil
}

func (c *diffBundleCommand) readModel(apiRoot base.APICallCloser) (*bundlechanges.Model, error) {
	status, err := c.getStatus(apiRoot)
	if err != nil {
		return nil, errors.Annotate(err, "getting model status")
	}
	model, err := appbundle.BuildModelRepresentation(status, c.makeModelExtractor(apiRoot), true, c.bundleMachines)
	return model, errors.Trace(err)
}

func (c *diffBundleCommand) getStatus(apiRoot base.APICallCloser) (*params.FullStatus, error) {
	// Ported from api.Client which is nigh impossible to test without
	// a real api.Connection.
	_, facade := base.NewClientFacade(apiRoot, "Client")
	var result params.FullStatus
	if err := facade.FacadeCall("FullStatus", params.StatusParams{}, &result); err != nil {
		return nil, errors.Trace(err)
	}
	// We don't care about model type.
	return &result, nil
}

func (c *diffBundleCommand) makeModelExtractor(apiRoot base.APICallCloser) appbundle.ModelExtractor {
	return &extractorImpl{
		application: application.NewClient(apiRoot),
		annotations: annotations.NewClient(apiRoot),
		modelConfig: modelconfig.NewClient(apiRoot),
	}
}

type extractorImpl struct {
	application *application.Client
	annotations *annotations.Client
	modelConfig *modelconfig.Client
}

// GetAnnotations is part of ModelExtractor.
func (e *extractorImpl) GetAnnotations(tags []string) ([]params.AnnotationsGetResult, error) {
	return e.annotations.Get(tags)
}

// GetConstraints is part of ModelExtractor.
func (e *extractorImpl) GetConstraints(applications ...string) ([]constraints.Value, error) {
	return e.application.GetConstraints(applications...)
}

// GetConfig is part of ModelExtractor.
func (e *extractorImpl) GetConfig(branchName string, applications ...string) ([]map[string]interface{}, error) {
	return e.application.GetConfig(branchName, applications...)
}

// Sequences is part of ModelExtractor.
func (e *extractorImpl) Sequences() (map[string]int, error) {
	return e.modelConfig.Sequences()
}

// BundleResolver defines what we need from a charm store to resolve a
// bundle and read the bundle data.
type BundleResolver interface {
	ResolveBundleURL(*charm.URL, commoncharm.Origin) (*charm.URL, commoncharm.Origin, error)
	GetBundle(*charm.URL, string) (charm.Bundle, error)
}
