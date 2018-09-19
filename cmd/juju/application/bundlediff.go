// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/bundlechanges"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charmrepo.v3"
	csparams "gopkg.in/juju/charmrepo.v3/csclient/params"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/annotations"
	"github.com/juju/juju/api/application"
	"github.com/juju/juju/api/modelconfig"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/constraints"
)

const bundleDiffDoc = `
Bundle can be a local bundle file or the name of a bundle in
the charm store. The bundle can also be combined with overlays (in the
same way as the deploy command) before comparing with the model.

The map-machines option works similarly as for the deploy command, but
existing is always assumed, so it doesn't need to be specified.

Examples:
    juju diff-bundle localbundle.yaml
    juju diff-bundle canonical-kubernetes
    juju diff-bundle mongodb-cluster --channel beta
    juju diff-bundle canonical-kubernetes --overlay local-config.yaml --overlay extra.yaml
    juju diff-bundle localbundle.yaml --map-machines 3=4

See also:
    deploy
`

// NewBundleDiffCommand returns a command to compare a bundle against
// the selected model.
func NewBundleDiffCommand() cmd.Command {
	return modelcmd.Wrap(&BundleDiffCommand{})
}

// BundleDiffCommand compares a bundle to a model.
type BundleDiffCommand struct {
	modelcmd.ModelCommandBase
	Bundle         string
	BundleOverlays []string
	Channel        csparams.Channel

	BundleMachines map[string]string
	MachineMap     string
}

// IsSuperCommand is part of cmd.Command.
func (c *BundleDiffCommand) IsSuperCommand() bool { return false }

// AllowInterspersedFlags is part of cmd.Command.
func (c *BundleDiffCommand) AllowInterspersedFlags() bool { return true }

// Info is part of cmd.Command.
func (c *BundleDiffCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "diff-bundle",
		Args:    "<bundle file or name>",
		Purpose: "Compare a bundle against a model and report any differences.",
		Doc:     bundleDiffDoc,
	}
}

// SetFlags is part of cmd.Command.
func (c *BundleDiffCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.StringVar((*string)(&c.Channel), "channel", "", "Channel to use when getting the bundle from the charm store")
	f.Var(cmd.NewAppendStringsValue(&c.BundleOverlays), "overlay", "Bundles to overlay on the primary bundle, applied in order")
	f.StringVar(&c.MachineMap, "map-machines", "", "Indicates how existing machines correspond to bundle machines")
}

// Init is part of cmd.Command.
func (c *BundleDiffCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("no bundle specified")
	}
	c.Bundle = args[0]
	// UseExisting is assumed for diffing.
	_, mapping, err := parseMachineMap(c.MachineMap)
	if err != nil {
		return errors.Annotate(err, "error in --map-machines")
	}
	c.BundleMachines = mapping

	return cmd.CheckEmpty(args[1:])
}

// Run is part of cmd.Command.
func (c *BundleDiffCommand) Run(ctx *cmd.Context) error {
	apiRoot, err := c.NewAPIRoot()
	if err != nil {
		return errors.Trace(err)
	}
	defer apiRoot.Close()

	// Load up the bundle data, with includes and overlays.
	bundle, bundleDir, err := c.readBundle(ctx, apiRoot)
	if err != nil {
		return errors.Trace(err)
	}
	if err := composeBundle(bundle, ctx, bundleDir, c.BundleOverlays); err != nil {
		return errors.Trace(err)
	}
	if err := verifyBundle(bundle, bundleDir); err != nil {
		return errors.Trace(err)
	}

	// Extract the information from the current model.
	model, err := c.readModel(apiRoot)
	if err != nil {
		return errors.Trace(err)
	}
	// Get the differences between them.
	diff, err := bundlechanges.BuildDiff(bundlechanges.DiffConfig{
		Bundle: bundle,
		Model:  model,
		Logger: logger,
	})

	if err != nil {
		return errors.Trace(err)
	}

	encoder := yaml.NewEncoder(ctx.Stdout)
	defer encoder.Close()
	err = encoder.Encode(diff)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (c *BundleDiffCommand) readBundle(ctx *cmd.Context, apiRoot api.Connection) (*charm.BundleData, string, error) {
	bundleData, bundleDir, err := readLocalBundle(ctx, c.Bundle)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	if bundleData != nil {
		return bundleData, bundleDir, nil
	}

	// Not a local bundle, so it must be from the charmstore.
	charmStore, err := c.charmStore()
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	bundleURL, _, err := resolveBundleURL(
		modelconfig.NewClient(apiRoot), charmStore, c.Bundle,
	)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	if bundleURL == nil {
		// This isn't a charmstore bundle either! Complain.
		return nil, "", errors.Errorf("couldn't interpret %q as a local or charmstore bundle", c.Bundle)
	}

	bundle, err := charmStore.GetBundle(bundleURL)
	if err != nil {
		return nil, "", errors.Trace(err)
	}

	return bundle.Data(), "", nil
}

func (c *BundleDiffCommand) charmStore() (*charmrepo.CharmStore, error) {
	controllerAPIRoot, err := c.NewControllerAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer controllerAPIRoot.Close()
	csURL, err := getCharmStoreAPIURL(controllerAPIRoot)
	if err != nil {
		return nil, errors.Trace(err)
	}
	bakeryClient, err := c.BakeryClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	cstoreClient := newCharmStoreClient(bakeryClient, csURL).WithChannel(c.Channel)
	return charmrepo.NewCharmStoreFromClient(cstoreClient), nil
}

func (c *BundleDiffCommand) readModel(apiRoot api.Connection) (*bundlechanges.Model, error) {
	status, err := apiRoot.Client().Status(nil)
	if err != nil {
		return nil, errors.Annotate(err, "getting model status")
	}
	model, err := buildModelRepresentation(status, c.makeModelExtractor(apiRoot), true, c.BundleMachines)
	return model, errors.Trace(err)
}

func (c *BundleDiffCommand) makeModelExtractor(apiRoot api.Connection) ModelExtractor {
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
func (e *extractorImpl) GetConfig(applications ...string) ([]map[string]interface{}, error) {
	return e.application.GetConfig(applications...)
}

// Sequences is part of ModelExtractor.
func (e *extractorImpl) Sequences() (map[string]int, error) {
	return e.modelConfig.Sequences()
}
