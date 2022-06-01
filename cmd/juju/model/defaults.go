// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package model

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"sort"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"
	"github.com/juju/schema"
	"github.com/juju/utils"
	"gopkg.in/yaml.v3"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	cloudapi "github.com/juju/juju/api/client/cloud"
	"github.com/juju/juju/api/client/modelmanager"
	jujucloud "github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/config"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	envconfig "github.com/juju/juju/environs/config"
)

const (
	modelDefaultsSummary = `Displays or sets default configuration settings for new models.`
	modelDefaultsHelpDoc = `
When run with no arguments, all default configuration (keys and values) are
displayed. Supplying a single default key returns the value for that key.
To set model defaults, you can supply one or more key=value pairs, or set
values from a yaml file using the --file flag.

Model default configuration settings are specific to the cloud on which the
model is deployed. If the controller hosts more than one cloud, the cloud
(and optionally region) must be specified using the --cloud flag. This flag
accepts arguments in the following forms:
    --cloud=<cloud>                    (specified cloud, all regions)
    --region=<region>               (default cloud, specified region)
    --region=<cloud>/<region>            (specified cloud and region)
    --cloud=<cloud> --region=<region>    (specified cloud and region)

Model defaults yaml configuration can be piped from stdin from the output of
the command stdout. Some model-defaults configuration are read-only; to prevent
the command exiting on read-only fields, use the --ignore-read-only-fields flag,
which will cause it to skip over these fields when they're encountered.

Examples:

Display all model config default values
    juju model-defaults

Display the value of http-proxy model config default
    juju model-defaults http-proxy

Display the value of http-proxy model config default for the aws cloud
    juju model-defaults --cloud=aws http-proxy

Display the value of http-proxy model config default for the aws cloud
and us-east-1 region
    juju model-defaults --region=aws/us-east-1 http-proxy

Display the value of http-proxy model config default for the us-east-1 region
    juju model-defaults --region=us-east-1 http-proxy

Set the value of ftp-proxy model config default to 10.0.0.1:8000
    juju model-defaults ftp-proxy=10.0.0.1:8000

Set the value of ftp-proxy model config default to 10.0.0.1:8000 in the
us-east-1 region
    juju model-defaults --region=us-east-1 ftp-proxy=10.0.0.1:8000

Set model default values for the aws cloud as defined in path/to/file.yaml
    juju model-defaults --cloud=aws --file path/to/file.yaml

Reset the value of default-series and test-mode to default
    juju model-defaults --reset default-series,test-mode

Reset the value of http-proxy for the us-east-1 region to default
    juju model-defaults --region us-east-1 --reset http-proxy

See also:
    models
    model-config
`
)

var defConfigBase = config.ConfigCommandBase{
	Resettable: true,
	CantReset:  []string{envconfig.AgentVersionKey},
}

// NewDefaultsCommand wraps defaultsCommand with sane model settings.
func NewDefaultsCommand() cmd.Command {
	defaultsCmd := &defaultsCommand{
		configBase: defConfigBase,
		newCloudAPI: func(caller base.APICallCloser) cloudAPI {
			return cloudapi.NewClient(caller)
		},
		newDefaultsAPI: func(caller base.APICallCloser) defaultsCommandAPI {
			return modelmanager.NewClient(caller)
		},
	}
	defaultsCmd.newAPIRoot = defaultsCmd.NewAPIRoot
	return modelcmd.WrapController(defaultsCmd)
}

type defaultAttrs map[string]interface{}

// CoerceFormat attempts to convert the defaultAttrs values from the complex
// type to the more simple type. This is because the output of this command
// outputs in the following format:
//
//     resource-name:
//        default: foo
//        controller: baz
//        regions:
//        - name: cloud-region-name
//          value: bar
//
// Where the consuming side of the command expects it in the following format:
//
//     resource-name: bar
//
// CoerceFormat attempts to diagnose this and attempt to do this correctly.
func (a defaultAttrs) CoerceFormat(region string) (defaultAttrs, error) {
	coerced := make(map[string]interface{})

	fields := schema.FieldMap(schema.Fields{
		"default":    schema.Any(),
		"controller": schema.Any(),
		"regions": schema.List(schema.FieldMap(schema.Fields{
			"name":  schema.String(),
			"value": schema.Any(),
		}, nil)),
	}, schema.Defaults{
		"controller": schema.Omit,
		"regions":    schema.Omit,
	})

	for k, v := range a {
		out, err := fields.Coerce(v, []string{})
		if err != nil {
			// Fallback to the old format and just pass through the value.
			coerced[k] = v
			continue
		}

		m := out.(map[string]interface{})
		v = m["default"]
		if ctrl, ok := m["controller"]; ok && region == "" {
			v = ctrl
		}
		if regions, ok := m["regions"].([]interface{}); ok && regions != nil {
			for _, r := range regions {
				regionMap, ok := r.(map[string]interface{})
				if !ok {
					continue
				}
				if regionMap["name"] == region {
					v = regionMap["value"]
				}
			}
		}

		// Resource tags in the new output format is a map[string]interface{},
		// but it should be of the format `foo=bar baz=boo`.
		if k == "resource-tags" {
			tags, err := coerceResourceTags(v)
			if err != nil {
				return nil, errors.Annotate(err, "unable to read resource-tags")
			}
			v = tags
		}

		coerced[k] = v
	}

	return coerced, nil
}

// defaultsCommand is compound command for accessing and setting attributes
// related to default model configuration.
type defaultsCommand struct {
	modelcmd.ControllerCommandBase
	configBase config.ConfigCommandBase
	out        cmd.Output

	newAPIRoot     func() (api.Connection, error)
	newDefaultsAPI func(base.APICallCloser) defaultsCommandAPI
	newCloudAPI    func(base.APICallCloser) cloudAPI

	// Extra `model-defaults`-specific fields
	cloud, regionFlag    string // `--cloud` and `--region` args
	region               string // parsed region
	ignoreReadOnlyFields bool
}

// cloudAPI defines an API to be passed in for testing.
type cloudAPI interface {
	Close() error
	Clouds() (map[names.CloudTag]jujucloud.Cloud, error)
	Cloud(names.CloudTag) (jujucloud.Cloud, error)
}

// defaultsCommandAPI defines an API to be used during testing.
type defaultsCommandAPI interface {
	// Close closes the api connection.
	Close() error

	// ModelDefaults returns the default config values used when creating a new model.
	ModelDefaults(cloud string) (envconfig.ModelDefaultAttributes, error)

	// SetModelDefaults sets the default config values to use
	// when creating new models.
	SetModelDefaults(cloud, region string, config map[string]interface{}) error

	// UnsetModelDefaults clears the default model
	// configuration values.
	UnsetModelDefaults(cloud, region string, keys ...string) error
}

// Info implements part of the cmd.Command interface.
func (c *defaultsCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Args:    "[[<cloud>/]<region> ]<model-key>[<=value>] ...]",
		Doc:     modelDefaultsHelpDoc,
		Name:    "model-defaults",
		Purpose: modelDefaultsSummary,
		Aliases: []string{"model-default"},
	})
}

// SetFlags implements part of the cmd.Command interface.
func (c *defaultsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ControllerCommandBase.SetFlags(f)
	c.configBase.SetFlags(f)

	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatDefaultConfigTabular,
	})
	f.BoolVar(&c.ignoreReadOnlyFields, "ignore-read-only-fields", false, "Ignore read only fields that might cause errors to be emitted while processing yaml documents")

	// The syntax here is consistent with the `add-k8s` command
	f.StringVar(&c.cloud, "cloud", "", "The cloud to target")
	f.StringVar(&c.regionFlag, "region", "", "The region or cloud/region to target")
}

// Init implements cmd.Command.Init.
func (c *defaultsCommand) Init(args []string) error {
	if c.regionFlag != "" {
		// Parse `regionFlag` into cloud and/or region
		splitCR := strings.SplitN(c.regionFlag, "/", 2)
		if len(splitCR) == 1 {
			// Only region specified
			c.region = splitCR[0]
		} else {
			// Cloud and region specified
			if c.cloud != "" {
				return errors.New(
					`cannot specify cloud using both --cloud and --region flags; use either
    --cloud=<cloud> --region=<region>
    --region=<cloud>/<region>`,
				)
			}
			c.cloud = splitCR[0]
			c.region = splitCR[1]
		}
	}

	// Check cloudName is syntactically valid
	if c.cloud != "" && !names.IsValidCloud(c.cloud) {
		return errors.NotValidf("cloud %q", c.cloud)
	}

	return c.configBase.Init(args)
}

// Run implements part of the cmd.Command interface.
func (c *defaultsCommand) Run(ctx *cmd.Context) error {
	root, err := c.newAPIRoot()
	if err != nil {
		return errors.Trace(err)
	}

	cc := c.newCloudAPI(root)
	defer cc.Close()
	err = c.validateCloudRegion(cc)
	if err != nil {
		return errors.Trace(err)
	}

	client := c.newDefaultsAPI(root)
	defer client.Close()

	for _, action := range c.configBase.Actions {
		var err error
		switch action {
		case config.GetOne:
			err = c.getDefaults(client, ctx)
		case config.Set:
			err = c.setDefaults(client)
		case config.SetFile:
			err = c.setDefaultsFile(client, ctx)
		case config.Reset:
			err = c.resetDefaults(client)
		default:
			err = c.getAllDefaults(client, ctx)
		}
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// validateCloudRegion checks that the supplied cloud and region is valid.
func (c *defaultsCommand) validateCloudRegion(cc cloudAPI) error {
	// If cloud not specified, set to default value
	if c.cloud == "" {
		// Try to set cloud to default
		cloudTag, err := c.maybeGetDefaultControllerCloud(cc)
		if err != nil {
			return errors.Trace(err)
		}
		c.cloud = cloudTag.Id()
	}

	// Check cloud exists
	cloud, err := cc.Cloud(names.NewCloudTag(c.cloud))
	if err != nil {
		return errors.Trace(err)
	}

	// If region specified: check it's valid
	if c.region != "" {
		regionValid := false
		for _, r := range cloud.Regions {
			if r.Name == c.region {
				regionValid = true
				break
			}
		}
		if !regionValid {
			return errors.Errorf("invalid region specified: %q", c.region)
		}
	}

	// All looks good!
	return nil
}

var noCloudMsg = `
You don't have access to any clouds on this controller.
Only controller administrators can set default model values.
`[1:]

var manyCloudsMsg = `
You haven't specified a cloud and more than one exists on this controller.
Specify one of the following clouds for which to process model defaults:
    %s
`[1:]

func (c *defaultsCommand) maybeGetDefaultControllerCloud(api cloudAPI) (names.CloudTag, error) {
	var cTag names.CloudTag
	clouds, err := api.Clouds()
	if err != nil {
		return cTag, errors.Trace(err)
	}
	if len(clouds) == 0 {
		return cTag, errors.New(noCloudMsg)
	}
	if len(clouds) != 1 {
		var cloudNames []string
		for _, c := range clouds {
			cloudNames = append(cloudNames, c.Name)
		}
		sort.Strings(cloudNames)
		return cTag, errors.Errorf(manyCloudsMsg, strings.Join(cloudNames, ","))
	}
	for cTag = range clouds {
		// Set cTag to the only cloud in the result.
	}
	return cTag, nil
}

// getDefaults writes out the value for a single default key.
func (c *defaultsCommand) getDefaults(client defaultsCommandAPI, ctx *cmd.Context) error {
	attrs, err := c.getFilteredDefaults(client)
	if err != nil {
		return errors.Trace(err)
	}

	if len(c.configBase.KeysToGet) == 0 {
		return errors.New("c.configBase.KeysToGet is empty")
	}
	if value, ok := attrs[c.configBase.KeysToGet[0]]; ok {
		return c.out.Write(ctx, envconfig.ModelDefaultAttributes{c.configBase.KeysToGet[0]: value})
	} else {
		msg := fmt.Sprintf("there are no default model values for %q", c.configBase.KeysToGet[0])
		if c.region != "" {
			msg += fmt.Sprintf(" in region %q", c.region)
		}
		return errors.New(msg)
	}
}

// getAllDefaults writes out the value for a single key or the full tree of
// defaults.
func (c *defaultsCommand) getAllDefaults(client defaultsCommandAPI, ctx *cmd.Context) error {
	attrs, err := c.getFilteredDefaults(client)
	if err != nil {
		return errors.Trace(err)
	}

	if c.region != "" && len(attrs) == 0 {
		return errors.New(fmt.Sprintf(
			"there are no default model values in region %q", c.region))
	}

	return c.out.Write(ctx, attrs)
}

// getFilteredDefaults returns model defaults, filtered by region if necessary.
func (c *defaultsCommand) getFilteredDefaults(client defaultsCommandAPI) (envconfig.ModelDefaultAttributes, error) {
	attrs, err := client.ModelDefaults(c.cloud)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	valueForRegion := func(region string, regions []envconfig.RegionDefaultValue) (envconfig.RegionDefaultValue, bool) {
		for _, r := range regions {
			if r.Name == region {
				return r, true
			}
		}
		return envconfig.RegionDefaultValue{}, false
	}

	// Filter by region if necessary.
	if c.region != "" {
		for attrName, attr := range attrs {
			if regionDefault, ok := valueForRegion(c.region, attr.Regions); !ok {
				delete(attrs, attrName)
			} else {
				attrForRegion := attr
				attrForRegion.Regions = []envconfig.RegionDefaultValue{regionDefault}
				attrs[attrName] = attrForRegion
			}
		}
	}

	return attrs, nil
}

// setDefaults sets defaults as provided by key=value command-line args.
func (c *defaultsCommand) setDefaults(client defaultsCommandAPI) error {
	// Have to convert c.configBase.ValsToSet to a map[string]interface{}
	attrs := make(map[string]interface{})
	for key, value := range c.configBase.ValsToSet {
		attrs[key] = value
	}
	coerced, err := c.handleAttrs(client, attrs)
	if err != nil {
		return errors.Trace(err)
	}
	return block.ProcessBlockedError(
		client.SetModelDefaults(
			c.cloud, c.region, coerced), block.BlockChange)
}

// setDefaultsFile sets defaults provided from a yaml file.
func (c *defaultsCommand) setDefaultsFile(client defaultsCommandAPI, ctx *cmd.Context) error {
	// Read file & unmarshal into yaml
	attrs := make(map[string]interface{})
	path, err := utils.NormalizePath(c.configBase.ConfigFile.Path)
	if err != nil {
		return errors.Trace(err)
	}
	data, err := ioutil.ReadFile(ctx.AbsPath(path))
	if err != nil {
		return errors.Trace(err)
	}
	if err := yaml.Unmarshal(data, &attrs); err != nil {
		return errors.Trace(err)
	}
	coerced, err := c.handleAttrs(client, attrs)
	if err != nil {
		return errors.Trace(err)
	}
	return block.ProcessBlockedError(
		client.SetModelDefaults(
			c.cloud, c.region, coerced), block.BlockChange)
}

// handleAttrs performs common logic for the set key methods - checking all
// keys are valid and settable, and coercing them to the correct format.
func (c *defaultsCommand) handleAttrs(client defaultsCommandAPI,
	attrs defaultAttrs) (defaultAttrs, error) {
	var keys []string
	values := make(defaultAttrs)
	for k, v := range attrs {
		if k == envconfig.AgentVersionKey {
			if c.ignoreReadOnlyFields {
				continue
			}
			return nil, errors.Errorf(`"agent-version" must be set via "upgrade-model"`)
		}
		values[k] = v
		keys = append(keys, k)
	}

	coerced, err := values.CoerceFormat(c.region)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err := c.verifyKnownKeys(client, keys); err != nil {
		return nil, errors.Trace(err)
	}
	return coerced, nil
}

// resetDefaults resets the keys in resetKeys.
func (c *defaultsCommand) resetDefaults(client defaultsCommandAPI) error {
	// ctx unused in this method.
	if err := c.verifyKnownKeys(client, c.configBase.KeysToReset); err != nil {
		return errors.Trace(err)
	}
	return block.ProcessBlockedError(
		client.UnsetModelDefaults(
			c.cloud, c.region, c.configBase.KeysToReset...), block.BlockChange)
}

// verifyKnownKeys is a helper to validate the keys we are operating with
// against the set of known attributes from the model.
func (c *defaultsCommand) verifyKnownKeys(client defaultsCommandAPI, keys []string) error {
	known, err := client.ModelDefaults(c.cloud)
	if err != nil {
		return errors.Trace(err)
	}

	allKeys := c.configBase.KeysToReset[:]
	for _, k := range keys {
		allKeys = append(allKeys, k)
	}

	for _, key := range allKeys {
		// check if the key exists in the known config
		// and warn the user if the key is not defined
		if _, exists := known[key]; !exists {
			logger.Warningf(
				"key %q is not defined in the known model configuration: possible misspelling", key)
		}
	}
	return nil
}

// formatConfigTabular writes a tabular summary of default config information.
func formatDefaultConfigTabular(writer io.Writer, value interface{}) error {
	defaultValues, ok := value.(envconfig.ModelDefaultAttributes)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", defaultValues, value)
	}

	tw := output.TabWriter(writer)
	w := output.Wrapper{
		TabWriter: tw,
	}

	p := func(name string, value envconfig.AttributeDefaultValues) {
		var c, d interface{}
		switch value.Default {
		case nil:
			d = "-"
		case "":
			d = `""`
		default:
			d = value.Default
		}
		switch value.Controller {
		case nil:
			c = "-"
		case "":
			c = `""`
		default:
			c = value.Controller
		}
		w.Println(name, d, c)
		for _, region := range value.Regions {
			w.Println("  "+region.Name, region.Value, "-")
		}
	}
	var valueNames []string
	for name := range defaultValues {
		valueNames = append(valueNames, name)
	}
	sort.Strings(valueNames)
	w.Println("Attribute", "Default", "Controller")

	for _, name := range valueNames {
		info := defaultValues[name]
		out := &bytes.Buffer{}
		err := cmd.FormatYaml(out, info)
		if err != nil {
			return errors.Annotatef(err, "formatting value for %q", name)
		}
		p(name, info)
	}

	tw.Flush()
	return nil
}
