// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"fmt"
	"io"
	"os"
	"strconv"

	charmresource "github.com/juju/charm/v13/resource"
	"github.com/juju/cmd/v4"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo/v2"
	"github.com/mattn/go-isatty"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/api/client/application"
	apicharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/cmd/modelcmd"
	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/resources"
)

var logger = loggo.GetLogger("juju.cmd.juju.application.utils")

// GetMetaResources retrieves metadata resources for the given
// charmURL.
func GetMetaResources(charmURL string, client CharmClient) (map[string]charmresource.Meta, error) {
	charmInfo, err := client.CharmInfo(charmURL)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return charmInfo.Meta.Resources, nil
}

// ParsePlacement validates provided placement of a unit and
// returns instance.Placement.
func ParsePlacement(spec string) (*instance.Placement, error) {
	if spec == "" {
		return nil, nil
	}
	placement, err := instance.ParsePlacement(spec)
	if err == instance.ErrPlacementScopeMissing {
		spec = fmt.Sprintf("model-uuid:%s", spec)
		placement, err = instance.ParsePlacement(spec)
	}
	if err != nil {
		return nil, errors.Errorf("invalid --to parameter %q", spec)
	}
	return placement, nil
}

// GetFlags returns the flags with the given names. Only flags that are set and
// whose name is included in flagNames are included.
func GetFlags(flagSet *gnuflag.FlagSet, flagNames []string) []string {
	flags := make([]string, 0, flagSet.NFlag())
	flagSet.Visit(func(flag *gnuflag.Flag) {
		for _, name := range flagNames {
			if flag.Name == name {
				flags = append(flags, flagWithMinus(name))
			}
		}
	})
	return flags
}

func flagWithMinus(name string) string {
	if len(name) > 1 {
		return "--" + name
	}
	return "-" + name
}

// GetUpgradeResources returns a map of resources which require
// refresh.
func GetUpgradeResources(
	newCharmID application.CharmID,
	repositoryResourceLister CharmClient,
	resourceLister ResourceLister,
	applicationID string,
	providedResources map[string]string,
	meta map[string]charmresource.Meta,
) (map[string]charmresource.Meta, error) {
	if len(meta) == 0 {
		return nil, nil
	}
	available, err := getAvailableRepositoryResources(newCharmID, repositoryResourceLister)
	if err != nil {
		return nil, errors.Trace(err)
	}
	current, err := getCurrentResources(applicationID, resourceLister)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return filterResourcesForUpgrade(newCharmID.Origin.Source, meta, current, available, providedResources)
}

// getCurrentResources gets the current resources for this charm in
// state.
func getCurrentResources(
	applicationID string,
	resourceLister ResourceLister,
) (map[string]resources.Resource, error) {
	svcs, err := resourceLister.ListResources([]string{applicationID})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return resources.AsMap(svcs[0].Resources), nil
}

// getAvailableRepositoryResources gets the current resources for this
// charm available in the repository.
func getAvailableRepositoryResources(newCharmID application.CharmID, repositoryResourceLister CharmClient) (map[string]charmresource.Resource, error) {
	if repositoryResourceLister == nil || !corecharm.CharmHub.Matches(newCharmID.Origin.Source.String()) {
		// not required for local charms
		return nil, nil
	}
	available, err := repositoryResourceLister.ListCharmResources(newCharmID.URL, newCharmID.Origin)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(available) == 0 {
		return nil, nil
	}
	availableResources := make(map[string]charmresource.Resource)
	for _, resource := range available {
		availableResources[resource.Name] = resource
	}
	return availableResources, nil
}

func filterResourcesForUpgrade(
	source apicharm.OriginSource,
	meta map[string]charmresource.Meta,
	current map[string]resources.Resource,
	available map[string]charmresource.Resource,
	providedResources map[string]string,
) (map[string]charmresource.Meta, error) {
	filtered := make(map[string]charmresource.Meta)
	for name, res := range meta {
		var doUpgrade bool
		var err error
		if source == apicharm.OriginLocal {
			doUpgrade, err = shouldUpgradeResourceLocalCharm(res.Name, providedResources, current)
		} else {
			doUpgrade, err = shouldUpgradeResource(res.Name, providedResources, current, available)
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		if doUpgrade {
			filtered[name] = res
		}
	}
	return filtered, nil
}

// shouldUpgradeResource reports whether we should upload the metadata for the given
// resource.
//
// Upgrade when:
//  1. Resources specified as a local file on the cli to be uploaded.
//  2. Resources specified as resource revision on the cli, which are
//     not currently in use.
//  3. Upstream has a newer resource available than what is currently used.
//  4. A new upstream resource is available.
//
// Caveat: Previously uploaded resources stay pinned to the data the user uploaded.
func shouldUpgradeResource(
	resName string,
	providedResources map[string]string,
	current map[string]resources.Resource,
	available map[string]charmresource.Resource,
) (bool, error) {

	cur, curFound := current[resName]
	providedResource, providedResourceFound := providedResources[resName]

	if providedResourceFound {
		providedResourceRev, err := strconv.Atoi(providedResource)
		if err == nil && curFound && cur.Revision == providedResourceRev && cur.Origin == charmresource.OriginStore {
			// A revision refers to resources in a repository. If the specified revision
			// is already uploaded, nothing to do.
			logger.Tracef("provided revision of %s resource already loaded", resName)
			return false, nil
		}
		logger.Tracef("%q provided to upgrade existing resource", resName)
		return true, nil
	}

	if !curFound {
		// If there's no information on the server, there might be a new resource added to the charm.
		logger.Tracef("resource %q does not exist in controller, so it will be uploaded", resName)
		return true, nil
	}

	avail, availFound := available[resName]
	if availFound &&
		avail.Revision == cur.Revision &&
		cur.Origin != charmresource.OriginUpload {
		logger.Tracef("available resource and current store resource have same revision, no upgrade")
		return false, nil
	}
	// Never override existing resources a user has already uploaded.
	return cur.Origin != charmresource.OriginUpload, nil
}

// shouldUpgradeResourceLocalCharm returns true if the resource provided as
// resName should be upgraded where a local charm is used. Only return true
// if a local file is provided for a resource known by the charm.
//
// resName is a resource name found in the charm metadata.
func shouldUpgradeResourceLocalCharm(
	name string,
	providedResources map[string]string,
	current map[string]resources.Resource,
) (bool, error) {
	_, curFound := current[name]
	providedResource, providedResourceFound := providedResources[name]
	switch {
	case !curFound && !providedResourceFound:
		// If there's no information on the server, there might be a new resource added to the charm.
		return false, errors.NewNotValid(nil, fmt.Sprintf("new resource %q was missing, please provide it via --resource", name))
	case curFound && providedResourceFound:
		_, err := strconv.Atoi(providedResource)
		if err != nil {
			// This is a filename to be uploaded.
			logger.Tracef("%q provided to upgrade existing resource", name)
			return true, nil
		} else {
			return false, errors.NewNotFound(nil, fmt.Sprintf("resource %q revision not found, provide via --resource", name))
		}
	case providedResourceFound:
		return true, nil
	}

	return false, nil
}

const maxValueSize = 5242880 // Max size for a config file.

// ReadValue reads the value of an option out of the named file.
// An empty content is valid, like in parsing the options. The upper
// size is 5M.
func ReadValue(ctx *cmd.Context, filesystem modelcmd.Filesystem, filename string) (string, error) {
	absFilename := ctx.AbsPath(filename)
	fi, err := filesystem.Stat(absFilename)
	if err != nil {
		return "", errors.Errorf("cannot read option from file %q: %v", filename, err)
	}
	if fi.Size() > maxValueSize {
		return "", errors.Errorf("size of option file is larger than 5M")
	}
	content, err := os.ReadFile(ctx.AbsPath(filename))
	if err != nil {
		return "", errors.Errorf("cannot read option from file %q: %v", filename, err)
	}
	return string(content), nil
}

// IsTerminal checks if the file descriptor is a terminal.
func IsTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}

	return isatty.IsTerminal(f.Fd())
}

type configFlag interface {
	// AbsoluteFileNames returns the absolute path of any file names specified.
	AbsoluteFileNames(ctx *cmd.Context) ([]string, error)

	// ReadConfigPairs returns just the k=v attributes
	ReadConfigPairs(ctx *cmd.Context) (map[string]interface{}, error)
}

// ProcessConfig processes the config defined by the config flag and returns
// the map of config values and any YAML file content.
// We may have a single file arg specified, in which case
// it points to a YAML file keyed on the charm name and
// containing values for any charm settings.
// We may also have key/value pairs representing
// charm settings which overrides anything in the YAML file.
// If more than one file is specified, that is an error.
func ProcessConfig(ctx *cmd.Context, filesystem modelcmd.Filesystem, configOptions configFlag, trust *bool) (map[string]string, string, error) {
	var configYAML []byte
	files, err := configOptions.AbsoluteFileNames(ctx)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	if len(files) > 1 {
		return nil, "", errors.Errorf("only a single config YAML file can be specified, got %d", len(files))
	}
	if len(files) == 1 {
		configYAML, err = os.ReadFile(files[0])
		if err != nil {
			return nil, "", errors.Trace(err)
		}
	}
	attr, err := configOptions.ReadConfigPairs(ctx)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	appConfig := make(map[string]string)
	for k, v := range attr {
		appConfig[k] = v.(string)

		// Handle @ syntax for including file contents as values so we
		// are consistent to how 'juju config' works
		if len(appConfig[k]) < 1 || appConfig[k][0] != '@' {
			continue
		}

		if appConfig[k], err = ReadValue(ctx, filesystem, appConfig[k][1:]); err != nil {
			return nil, "", errors.Trace(err)
		}
	}

	// Expand the trust flag into the appConfig
	if trust != nil {
		appConfig[coreapplication.TrustConfigOptionName] = strconv.FormatBool(*trust)
	}
	return appConfig, string(configYAML), nil
}

// CombinedConfig takes the config key/value pairs and config.yaml file
// and combines into a yaml string. The key/value pairs representing
// charm settings overrides anything in the YAML file. If more than one
// file is specified, that is an error.
func CombinedConfig(ctx *cmd.Context, filesystem modelcmd.Filesystem, configOptions configFlag, appName string) (string, error) {
	settings, err := settingsFromYaml(ctx, configOptions, appName)
	if err != nil {
		return "", errors.Trace(err)
	}
	attr, err := configOptions.ReadConfigPairs(ctx)
	if err != nil {
		return "", errors.Trace(err)
	}
	appConfig := make(map[string]string)
	for k, v := range attr {
		appConfig[k] = v.(string)

		// Handle @ syntax for including file contents as values so we
		// are consistent to how 'juju config' works
		if len(appConfig[k]) < 1 || appConfig[k][0] != '@' {
			continue
		}

		if appConfig[k], err = ReadValue(ctx, filesystem, appConfig[k][1:]); err != nil {
			return "", errors.Trace(err)
		}
	}
	// CLI config over-rides config in the yaml file.
	for k, v := range appConfig {
		settings[k] = v
	}
	// Return to the expected yaml format for over the wire
	settingsForYaml := map[interface{}]interface{}{appName: settings}
	configYaml, err := goyaml.Marshal(settingsForYaml)
	if err != nil {
		return "", errors.Trace(err)
	}
	return string(configYaml), nil
}

func settingsFromYaml(ctx *cmd.Context, configOptions configFlag, appName string) (map[interface{}]interface{}, error) {
	var configYAML []byte
	files, err := configOptions.AbsoluteFileNames(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	switch len(files) {
	case 0:
		return make(map[interface{}]interface{}), nil
	case 1:
		configYAML, err = os.ReadFile(files[0])
		if err != nil {
			return nil, errors.Trace(err)
		}
	default:
		return nil, errors.Errorf("only a single config YAML file can be specified, got %d", len(files))
	}
	var allSettings map[string]interface{}
	if err := goyaml.Unmarshal(configYAML, &allSettings); err != nil {
		return nil, errors.Annotate(err, "cannot parse settings data")
	}
	settings, ok := allSettings[appName].(map[interface{}]interface{})
	if !ok {
		return nil, errors.Trace(err)
	}
	return settings, nil
}

func CheckFile(name, path string, fs modelcmd.Filesystem) error {
	_, err := fs.Stat(path)
	if os.IsNotExist(err) {
		return errors.Annotatef(err, "file for resource %q", name)
	}
	if err != nil {
		return errors.Annotatef(err, "can't read file for resource %q", name)
	}
	return nil
}
