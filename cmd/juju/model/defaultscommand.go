// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package model

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	cloudapi "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/api/modelmanager"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/environs/config"
)

const (
	modelDefaultsSummary = `Displays or sets default configuration settings for a model.`
	modelDefaultsHelpDoc = `
By default, all default configuration (keys and values) are
displayed if a key is not specified. Supplying key=value will set the
supplied key to the supplied value. This can be repeated for multiple keys.
You can also specify a yaml file containing key values.
By default, the model is the current model.


Examples:
    juju model-defaults
    juju model-defaults http-proxy
    juju model-defaults aws/us-east-1 http-proxy
    juju model-defaults us-east-1 http-proxy
    juju model-defaults -m mymodel type
    juju model-defaults ftp-proxy=10.0.0.1:8000
    juju model-defaults aws/us-east-1 ftp-proxy=10.0.0.1:8000
    juju model-defaults us-east-1 ftp-proxy=10.0.0.1:8000
    juju model-defaults us-east-1 ftp-proxy=10.0.0.1:8000 path/to/file.yaml
    juju model-defaults us-east-1 path/to/file.yaml    
    juju model-defaults -m othercontroller:mymodel default-series=yakkety test-mode=false
    juju model-defaults --reset default-series test-mode
    juju model-defaults aws/us-east-1 --reset http-proxy
    juju model-defaults us-east-1 --reset http-proxy

See also:
    models
    model-config
`
)

// NewDefaultsCommand wraps defaultsCommand with sane model settings.
func NewDefaultsCommand() cmd.Command {
	defaultsCmd := &defaultsCommand{
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

// defaultsCommand is compound command for accessing and setting attributes
// related to default model configuration.
type defaultsCommand struct {
	out cmd.Output
	modelcmd.ControllerCommandBase

	newAPIRoot     func() (api.Connection, error)
	newDefaultsAPI func(base.APICallCloser) defaultsCommandAPI
	newCloudAPI    func(base.APICallCloser) cloudAPI

	// args holds all the command-line arguments before
	// they've been parsed.
	args []string

	action                func(defaultsCommandAPI, *cmd.Context) error // The function handling the input, set in Init.
	key                   string
	resetKeys             []string // Holds the keys to be reset once parsed.
	cloudName, regionName string
	reset                 []string // Holds the keys to be reset until parsed.
	setOptions            common.ConfigFlag
}

// cloudAPI defines an API to be passed in for testing.
type cloudAPI interface {
	Close() error
	DefaultCloud() (names.CloudTag, error)
	Cloud(names.CloudTag) (jujucloud.Cloud, error)
}

// defaultsCommandAPI defines an API to be used during testing.
type defaultsCommandAPI interface {
	// Close closes the api connection.
	Close() error

	// ModelDefaults returns the default config values used when creating a new model.
	ModelDefaults() (config.ModelDefaultAttributes, error)

	// SetModelDefaults sets the default config values to use
	// when creating new models.
	SetModelDefaults(cloud, region string, config map[string]interface{}) error

	// UnsetModelDefaults clears the default model
	// configuration values.
	UnsetModelDefaults(cloud, region string, keys ...string) error
}

// Info implements part of the cmd.Command interface.
func (c *defaultsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Args:    "[[<cloud/>]<region> ]<model-key>[<=value>] ...]",
		Doc:     modelDefaultsHelpDoc,
		Name:    "model-defaults",
		Purpose: modelDefaultsSummary,
		Aliases: []string{"model-default"},
	}
}

// SetFlags implements part of the cmd.Command interface.
func (c *defaultsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ControllerCommandBase.SetFlags(f)

	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatDefaultConfigTabular,
	})
	f.Var(cmd.NewAppendStringsValue(&c.reset), "reset", "Reset the provided comma delimited keys")
}

// Init implements cmd.Command.Init.
func (c *defaultsCommand) Init(args []string) error {
	// There's no way of distinguishing a cloud name
	// from a model configuration setting without contacting the
	// API, but we aren't allowed to contact the API at Init time,
	// so we defer parsing the arguments until Run is called.
	c.args = args
	return nil
}

// Run implements part of the cmd.Command interface.
func (c *defaultsCommand) Run(ctx *cmd.Context) error {
	if err := c.parseArgs(c.args); err != nil {
		return errors.Trace(err)
	}
	root, err := c.newAPIRoot()
	if err != nil {
		return errors.Trace(err)
	}
	client := c.newDefaultsAPI(root)
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()

	if len(c.resetKeys) > 0 {
		err := c.resetDefaults(client, ctx)
		if err != nil {
			// We return this error naked as it is almost certainly going to be
			// cmd.ErrSilent and the cmd.Command framework expects that back
			// from cmd.Run if the process is blocked.
			return err
		}
	}
	if c.action == nil {
		// If we are reset only we end up here, only we've already done that.
		return nil
	}
	return c.action(client, ctx)
}

// This needs to parse a command line invocation to reset and set, or get
// model-default values. The arguments may be interspersed as demonstrated in
// the examples.
//
// This sets foo=baz and unsets bar in aws/us-east-1
//     juju model-defaults aws/us-east-1 foo=baz --reset bar
//
// If aws is the cloud of the current or specified controller -- specified by
// -c somecontroller -- then the following would also be equivalent.
//     juju model-defaults --reset bar us-east-1 foo=baz
//
// If one doesn't specify a cloud or region the command is still valid but for
// setting the default on the controller:
//     juju model-defaults foo=baz --reset bar
//
// Of course one can specify multiple keys to reset --reset a,b,c and one can
// also specify multiple values to set a=b c=d e=f. I.e. comma separated for
// resetting and space separated for setting. One may also only set or reset as
// a singular action.
//     juju model-defaults --reset foo
//     juju model-defaults a=b c=d e=f
//     juju model-defaults a=b c=d --reset e,f
//
// cloud/region may also be specified so above examples with that option might
// be like the following invokation.
//     juju model-defaults us-east-1 a=b c=d --reset e,f
//
// Finally one can also ask for the all the defaults or the defaults for one
// specific setting. In this case specifying a region is not valid as
// model-defaults shows the settings for a value at all locations that it has a
// default set -- or at a minimum the default and  "-" for a controller with no
// value set.
//     juju model-defaults
//     juju model-defaults no-proxy
//
// It is not valid to reset and get or to set and get values. It is also
// neither valid to reset and set the same key, nor to set the same key to
// different values in the same command.
//
// For those playing along that all means the first positional arg can be a
// cloud/region, a region, a key=value to set, a key to get the settings for,
// or empty. Other caveats are that one cannot set and reset a value for the
// same key, that is to say keys to be mutated must be unique.
//
// Here we go...
func (c *defaultsCommand) parseArgs(args []string) error {
	var err error
	//  If there's nothing to reset and no args we're returning everything. So
	//  we short circuit immediately.
	if len(args) == 0 && len(c.reset) == 0 {
		c.action = c.getDefaults
		return nil
	}

	// If there is an argument provided to reset, we turn it into a slice of
	// strings and verify them. If there is one or more valid keys to reset and
	// no other errors initializing the command, c.resetDefaults will be called
	// in c.Run.
	if err = c.parseResetKeys(); err != nil {
		return errors.Trace(err)
	}

	// Look at the first positional arg and test to see if it is a valid
	// optional specification of cloud/region or region. If it is then
	// cloudName and regionName are set on the object and the positional args
	// are returned without the first element. If it cannot be validated;
	// cloudName and regionName are left empty and we get back the same args we
	// passed in.
	args, err = c.parseArgsForRegion(args)
	if err != nil {
		return errors.Trace(err)
	}

	// Remember we *might* have one less arg at this point if we chopped the
	// first off because it was a valid cloud/region option.
	wantSet := false
	if len(args) > 0 {
		lastArg := args[len(args)-1]
		// We may have a config.yaml file
		_, err := os.Stat(lastArg)
		wantSet = err == nil || strings.Contains(lastArg, "=")
	}

	switch {
	case wantSet:
		// In the event that we are setting values, the final positional arg
		// will always have an "=" in it. So if we see that we know we want to
		// set args.
		return c.handleSetArgs(args)
	case len(args) == 0:
		if len(c.resetKeys) == 0 {
			// If there's no positional args and reset is not set then we're
			// getting all attrs.
			c.action = c.getDefaults
			return nil
		}
		// Reset only.
		return nil
	case len(args) == 1:
		// We want to get settings for the provided key.
		return c.handleOneArg(args[0])
	default: // case args > 1
		// Specifying any non key=value positional args after a key=value pair
		// is invalid input. So if we have more than one the input is almost
		// certainly invalid, but in different possible ways.
		return c.handleExtraArgs(args)
	}
}

// parseResetKeys splits the keys provided to --reset after trimming any
// leading or trailing comma. It then verifies that we haven't incorrectly
// received any key=value pairs and finally sets the value(s) on c.resetKeys.
func (c *defaultsCommand) parseResetKeys() error {
	if len(c.reset) == 0 {
		return nil
	}
	var resetKeys []string
	for _, value := range c.reset {
		keys := strings.Split(strings.Trim(value, ","), ",")
		resetKeys = append(resetKeys, keys...)
	}

	for _, k := range resetKeys {
		if k == config.AgentVersionKey {
			return errors.Errorf("%q cannot be reset", config.AgentVersionKey)
		}
		if strings.Contains(k, "=") {
			return errors.Errorf(
				`--reset accepts a comma delimited set of keys "a,b,c", received: %q`, k)
		}
	}
	c.resetKeys = resetKeys
	return nil
}

// parseArgsForRegion parses args to check if the first arg is a region and
// returns the appropriate remaining args.
func (c *defaultsCommand) parseArgsForRegion(args []string) ([]string, error) {
	var err error
	if len(args) > 0 {
		// determine if the first arg is cloud/region or region and return
		// appropriate positional args.
		args, err = c.parseCloudRegion(args)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	return args, nil
}

// parseCloudRegion examines args to see if the first arg is a cloud/region or
// region. If not it returns the full args slice. If it is then it sets cloud
// and/or region on the object and sends the remaining args back to the caller.
func (c *defaultsCommand) parseCloudRegion(args []string) ([]string, error) {
	var cloud, region string
	cr := args[0]
	// Must have no more than one slash and it must not be at the beginning or end.
	if strings.Count(cr, "/") == 1 && !strings.HasPrefix(cr, "/") && !strings.HasSuffix(cr, "/") {
		elems := strings.Split(cr, "/")
		cloud, region = elems[0], elems[1]
	} else {
		region = cr
	}

	// TODO(redir) 2016-10-05 #1627162
	// We don't disallow "=" in region names, but probably should.
	if strings.Contains(region, "=") {
		return args, nil
	}

	valid, err := c.validCloudRegion(cloud, region)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !valid {
		return args, nil
	}
	return args[1:], nil
}

// validCloudRegion checks that region is a valid region in cloud, or default cloud
// if cloud is not specified.
func (c *defaultsCommand) validCloudRegion(cloudName, region string) (bool, error) {
	var (
		isCloudRegion bool
		cloud         jujucloud.Cloud
		cTag          names.CloudTag
		err           error
	)

	root, err := c.newAPIRoot()
	if err != nil {
		return false, errors.Trace(err)
	}
	cc := c.newCloudAPI(root)
	defer cc.Close()

	if cloudName == "" {
		cTag, err = cc.DefaultCloud()
		if err != nil {
			return false, errors.Trace(err)
		}
	} else {
		if !names.IsValidCloud(cloudName) {
			return false, errors.Errorf("invalid cloud %q", cloudName)
		}
		cTag = names.NewCloudTag(cloudName)
	}
	cloud, err = cc.Cloud(cTag)
	if err != nil {
		return false, errors.Trace(err)
	}

	for _, r := range cloud.Regions {
		if r.Name == region {
			c.cloudName = cTag.Id()
			c.regionName = region
			isCloudRegion = true
			break
		}
	}
	return isCloudRegion, nil
}

// handleSetArgs parses args for setting defaults.
func (c *defaultsCommand) handleSetArgs(args []string) error {
	// We may have a config.yaml file
	_, err := os.Stat(args[0])
	argZeroKeyOnly := err != nil && !strings.Contains(args[0], "=")
	// If an invalid region was specified, the first positional arg won't have
	// an "=". If we see one here we know it is invalid.
	switch {
	case argZeroKeyOnly && c.regionName == "":
		return errors.Errorf("invalid region specified: %q", args[0])
	case argZeroKeyOnly && c.regionName != "":
		return errors.New("cannot set and retrieve default values simultaneously")
	default:
		if err := c.parseSetKeys(args); err != nil {
			return errors.Trace(err)
		}
		c.action = c.setDefaults
		return nil
	}
}

// parseSetKeys iterates over the args and make sure that the key=value pairs
// are valid. It also checks that the same key isn't also being reset.
func (c *defaultsCommand) parseSetKeys(args []string) error {
	for _, arg := range args {
		if err := c.setOptions.Set(arg); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// handleOneArg handles the case where we have one positional arg after
// processing for a region and the reset flag.
func (c *defaultsCommand) handleOneArg(arg string) error {
	resetSpecified := c.resetKeys != nil
	regionSpecified := c.regionName != ""

	if regionSpecified {
		if resetSpecified {
			// If a region was specified and reset was specified, we shouldn't have
			// an extra arg. If it had an "=" in it, we should have handled it
			// already.
			return errors.New("cannot retrieve defaults for a region and reset attributes at the same time")
		}
	}
	if resetSpecified {
		// It makes no sense to supply a positional arg that isn't a region if
		// we are resetting keys in a region, so we must have gotten an invalid
		// region.
		return errors.Errorf("invalid region specified: %q", arg)
	}
	// We can retrieve a value.
	c.key = arg
	c.action = c.getDefaults
	return nil
}

// handleExtraArgs handles the case where too many args were supplied.
func (c *defaultsCommand) handleExtraArgs(args []string) error {
	resetSpecified := c.resetKeys != nil
	regionSpecified := c.regionName != ""
	numArgs := len(args)

	// if we have a key=value pair here then something is wrong because the
	// last positional arg is not one. We assume the user intended to get a
	// value after setting them.
	for _, arg := range args {
		// We may have a config.yaml file
		_, err := os.Stat(arg)
		if err == nil || strings.Contains(arg, "=") {
			return errors.New("cannot set and retrieve default values simultaneously")
		}
	}

	if !regionSpecified {
		if resetSpecified {
			if numArgs == 2 {
				// It makes no sense to supply a positional arg that isn't a
				// region if we are resetting a region, so we must have gotten
				// an invalid region.
				return errors.Errorf("invalid region specified: %q", args[0])
			}
		}
		if !resetSpecified {
			// If we land here it is because there are extraneous positional
			// args.
			return errors.New("can only retrieve defaults for one key or all")
		}
	}
	return errors.New("invalid input")
}

// getDefaults writes out the value for a single key or the full tree of
// defaults.
func (c *defaultsCommand) getDefaults(client defaultsCommandAPI, ctx *cmd.Context) error {
	attrs, err := client.ModelDefaults()
	if err != nil {
		return err
	}

	valueForRegion := func(region string, regions []config.RegionDefaultValue) (config.RegionDefaultValue, bool) {
		for _, r := range regions {
			if r.Name == region {
				return r, true
			}
		}
		return config.RegionDefaultValue{}, false
	}

	// Filter by region if necessary.
	if c.regionName != "" {
		for attrName, attr := range attrs {
			if regionDefault, ok := valueForRegion(c.regionName, attr.Regions); !ok {
				delete(attrs, attrName)
			} else {
				attrForRegion := attr
				attrForRegion.Regions = []config.RegionDefaultValue{regionDefault}
				attrs[attrName] = attrForRegion
			}
		}
	}

	if c.key != "" {
		if value, ok := attrs[c.key]; ok {
			attrs = config.ModelDefaultAttributes{
				c.key: value,
			}
		} else {
			msg := fmt.Sprintf("there are no default model values for %q", c.key)
			if c.regionName != "" {
				msg += fmt.Sprintf(" in region %q", c.regionName)
			}
			return errors.New(msg)
		}
	}
	if c.regionName != "" && len(attrs) == 0 {
		return errors.New(fmt.Sprintf(
			"there are no default model values in region %q", c.regionName))
	}

	// If c.keys is empty, write out the whole lot.
	return c.out.Write(ctx, attrs)
}

// setDefaults sets defaults as provided in c.values.
func (c *defaultsCommand) setDefaults(client defaultsCommandAPI, ctx *cmd.Context) error {
	attrs, err := c.setOptions.ReadAttrs(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	var keys []string
	values := make(attributes)
	for k, v := range attrs {
		if k == config.AgentVersionKey {
			return errors.Errorf(`"agent-version" must be set via "upgrade-model"`)
		}
		values[k] = v
		keys = append(keys, k)
	}

	for _, k := range c.resetKeys {
		if _, ok := values[k]; ok {
			return errors.Errorf(
				"key %q cannot be both set and unset in the same command", k)
		}
	}

	if err := c.verifyKnownKeys(client, keys); err != nil {
		return errors.Trace(err)
	}
	return block.ProcessBlockedError(
		client.SetModelDefaults(
			c.cloudName, c.regionName, values), block.BlockChange)
}

// resetDefaults resets the keys in resetKeys.
func (c *defaultsCommand) resetDefaults(client defaultsCommandAPI, ctx *cmd.Context) error {
	// ctx unused in this method.
	if err := c.verifyKnownKeys(client, c.resetKeys); err != nil {
		return errors.Trace(err)
	}
	return block.ProcessBlockedError(
		client.UnsetModelDefaults(
			c.cloudName, c.regionName, c.resetKeys...), block.BlockChange)

}

// verifyKnownKeys is a helper to validate the keys we are operating with
// against the set of known attributes from the model.
func (c *defaultsCommand) verifyKnownKeys(client defaultsCommandAPI, keys []string) error {
	known, err := client.ModelDefaults()
	if err != nil {
		return errors.Trace(err)
	}

	allKeys := c.resetKeys[:]
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
	defaultValues, ok := value.(config.ModelDefaultAttributes)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", defaultValues, value)
	}

	tw := output.TabWriter(writer)
	w := output.Wrapper{tw}

	p := func(name string, value config.AttributeDefaultValues) {
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
