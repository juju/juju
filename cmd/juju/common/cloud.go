// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/environschema.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/jujuclient"
)

var logger = loggo.GetLogger("juju.cmd.juju.common")

type chooseCloudRegionError struct {
	error
}

// IsChooseCloudRegionError reports whether or not the given
// error was returned from ChooseCloudRegion.
func IsChooseCloudRegionError(err error) bool {
	_, ok := errors.Cause(err).(chooseCloudRegionError)
	return ok
}

// CloudOrProvider finds and returns cloud or provider.
func CloudOrProvider(cloudName string, cloudByNameFunc func(string) (*jujucloud.Cloud, error)) (cloud *jujucloud.Cloud, err error) {
	if cloud, err = cloudByNameFunc(cloudName); err != nil {
		if !errors.IsNotFound(err) {
			return nil, err
		}
		builtInClouds, err := BuiltInClouds()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if builtIn, ok := builtInClouds[cloudName]; !ok {
			return nil, errors.NotValidf("cloud %v", cloudName)
		} else {
			cloud = &builtIn
		}
	}
	return cloud, nil
}

// ChooseCloudRegion returns the cloud.Region to use, based on the specified
// region name. If no region name is specified, and there is at least one
// region, we use the first region in the list. If there are no regions, then
// we return a region with no name, having the same endpoints as the cloud.
func ChooseCloudRegion(cloud jujucloud.Cloud, regionName string) (jujucloud.Region, error) {
	if regionName != "" {
		region, err := jujucloud.RegionByName(cloud.Regions, regionName)
		if err != nil {
			return jujucloud.Region{}, errors.Trace(chooseCloudRegionError{err})
		}
		return *region, nil
	}
	if len(cloud.Regions) > 0 {
		// No region was specified, use the first region in the list.
		return cloud.Regions[0], nil
	}
	return jujucloud.Region{
		"", // no region name
		cloud.Endpoint,
		cloud.IdentityEndpoint,
		cloud.StorageEndpoint,
	}, nil
}

// BuiltInClouds returns cloud information for those
// providers which are built in to Juju.
func BuiltInClouds() (map[string]jujucloud.Cloud, error) {
	allClouds := make(map[string]jujucloud.Cloud)
	for _, providerType := range environs.RegisteredProviders() {
		p, err := environs.Provider(providerType)
		if err != nil {
			return nil, errors.Trace(err)
		}
		detector, ok := p.(environs.CloudDetector)
		if !ok {
			continue
		}
		clouds, err := detector.DetectClouds()
		if err != nil {
			return nil, errors.Annotatef(
				err, "detecting clouds for provider %q",
				providerType,
			)
		}
		for _, cloud := range clouds {
			allClouds[cloud.Name] = cloud
		}
	}
	return allClouds, nil
}

// CloudByName returns a cloud for given name
// regardless of whether it's public, private or builtin cloud.
// Not to be confused with cloud.CloudByName which does not cater
// for built-in clouds like localhost.
func CloudByName(cloudName string) (*jujucloud.Cloud, error) {
	cloud, err := jujucloud.CloudByName(cloudName)
	if err != nil {
		if errors.IsNotFound(err) {
			// Check built in clouds like localhost (lxd).
			builtinClouds, err := BuiltInClouds()
			if err != nil {
				return nil, errors.Trace(err)
			}
			aCloud, found := builtinClouds[cloudName]
			if !found {
				return nil, errors.NotFoundf("cloud %s", cloudName)
			}
			return &aCloud, nil
		}
		return nil, errors.Trace(err)
	}
	return cloud, nil
}

// CloudSchemaByType returns the Schema for a given cloud type.
// If the ProviderSchema is not implemented for the given cloud
// type, a NotFound error is returned.
func CloudSchemaByType(cloudType string) (environschema.Fields, error) {
	provider, err := environs.Provider(cloudType)
	if err != nil {
		return nil, err
	}
	ps, ok := provider.(environs.ProviderSchema)
	if !ok {
		return nil, errors.NotImplementedf("environs.ProviderSchema")
	}
	providerSchema := ps.Schema()
	if providerSchema == nil {
		return nil, errors.New("Failed to retrieve Provider Schema")
	}
	return providerSchema, nil
}

// ProviderConfigSchemaSourceByType returns a config.ConfigSchemaSource
// for the environ provider, found for the given cloud type, or an error.
func ProviderConfigSchemaSourceByType(cloudType string) (config.ConfigSchemaSource, error) {
	provider, err := environs.Provider(cloudType)
	if err != nil {
		return nil, err
	}
	if cs, ok := provider.(config.ConfigSchemaSource); ok {
		return cs, nil
	}
	return nil, errors.NotImplementedf("config.ConfigSource")
}

// PrintConfigSchema is used to print model configuration schema.
type PrintConfigSchema struct {
	Type        string `yaml:"type,omitempty" json:"type,omitempty"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

func FormatConfigSchema(values interface{}) (string, error) {
	out := &bytes.Buffer{}
	err := cmd.FormatSmart(out, values)
	if err != nil {
		return "", err
	}
	return out.String(), nil
}

// CloudAPI specifies what can be obtained from the controller.
type CloudAPI interface {
	// Clouds lists all clouds that currently logged in user has permissions to use/see on the controller.
	Clouds() (map[names.CloudTag]jujucloud.Cloud, error)

	// Cloud returns a remote cloud for the provided tag.
	Cloud(names.CloudTag) (jujucloud.Cloud, error)
}

// UploadAPI provides means of uploading remote entities to the controller.
type UploadAPI interface {
	// AddCloud uploads cloud to the controller.
	AddCloud(jujucloud.Cloud) error
}

// ParseCloudRegion parses and verifies supplied 'cloud/region' string.
// Verification involves ensuring that:
// * local <cloud> exists
// * or a remote <cloud> exists and currently logged on user has permissions to use it;
// * <region> is the valid region for supplied <cloud>.
func ParseCloudRegion(given CloudRegionValidationParams) (cloudTag names.CloudTag, cloud jujucloud.Cloud, cloudRegion string, err error) {
	return ParseCloudRegionMaybeDefaultRegion(given)
}

// CloudRegionValidationParams holds parameters to fine-tune cloud/region validation.
type CloudRegionValidationParams struct {
	// CloudRegion given cloud/region to validate.
	CloudRegion string

	// LocalStore holds the store to examine locally stored credentials.
	LocalStore jujuclient.CredentialGetter

	// LocalOnly indicates that this validation should only validate against local clouds.
	LocalOnly bool

	// RemoteCloudClient is the client that can query controller for remote clouds information.
	RemoteCloudClient CloudAPI

	// RemoteOnly indicates that this validation should only validate against remote clouds.
	RemoteOnly bool

	// Command is the command name that requested validation.
	Command string

	// UseDefaultRegion instructs to use default region if none specified.
	UseDefaultRegion bool
}

// ParseCloudRegionMaybeDefaultRegion parses and verifies supplied 'cloud/region' string.
// The method attempts to find remote cloud first, if cloudClient is supplied,
// and then looks for a cloud locally if it is not found.
// If region has not been passed in, this method may return default or first found
//cloud region, if the default is not set locally or the cloud is remote, based on
// the maybeDefaultRegion value.
// Verification involves ensuring that:
// * local <cloud> exists
// * or a remote <cloud> exists and currently logged on user has permissions to use it;
// * <region> is the valid region for supplied <cloud>.
func ParseCloudRegionMaybeDefaultRegion(given CloudRegionValidationParams) (cloudTag names.CloudTag, cloud jujucloud.Cloud, cloudRegion string, err error) {
	fail := func(err error) (names.CloudTag, jujucloud.Cloud, string, error) {
		return names.CloudTag{}, jujucloud.Cloud{}, "", err
	}
	remote := false
	getCloud := func(name string) (*jujucloud.Cloud, error) {
		if !given.LocalOnly {
			cloudTag = names.NewCloudTag(name)
			remoteCloud, err := given.RemoteCloudClient.Cloud(cloudTag)
			if err == nil {
				remote = true
				return &remoteCloud, nil
			}
			if !params.IsCodeNotFound(err) {
				return nil, err
			} else if given.RemoteOnly {
				// If remote cloud is not found and
				// we were told to only look for a remote cloud, err out here.
				return nil, errors.NotFoundf("remote cloud %v", name)
			}
		}
		// Look for a local cloud.
		return CloudByName(name)
	}

	cloudName, cloudRegion, err := jujucloud.SplitHostCloudRegion(given.CloudRegion)
	if err != nil {
		return fail(errors.Trace(err))
	}
	if cloudName != "" && cloudRegion != "" {
		// User specified "cloud/region".
		if !names.IsValidCloud(cloudName) {
			return fail(errors.NotValidf("cloud name %q", cloudName))
		}
		foundCloud, err := getCloud(cloudName)
		if err != nil {
			return fail(errors.Trace(err))
		}
		cloud = *foundCloud
	} else {
		// User specified "cloud" or "region". We'll try first
		// for cloud (check if it's a valid cloud name, and
		// whether there is a cloud by that name), and then
		// as a region within the default cloud.
		if cloudName != "" && names.IsValidCloud(cloudName) {
			foundCloud, err := getCloud(cloudName)
			if err == nil {
				cloud = *foundCloud
			} else if !errors.IsNotFound(err) {
				return fail(errors.Trace(err))
			} else {
				// We could not find a cloud with the specified name,
				// so we'll try the name as a region in the controller's cloud.
				cloudRegion, cloudName = cloudName, ""
			}
		} else {
			// We could not find a cloud with the specified name,
			// so we'll try the name as a region in the controller's cloud.
			cloudRegion, cloudName = cloudName, ""
		}
	}
	if cloudName == "" && !given.LocalOnly {
		cloudTag, cloud, err = MaybeGetControllerCloud(given.RemoteCloudClient, given.Command)
		if err != nil {
			return fail(errors.Trace(err))
		}
		remote = true
	}

	// microk8s is special.
	specialMk8s := cloud.Type == caas.K8sCloudMicrok8s && cloudRegion == caas.Microk8sRegion
	if cloudRegion != "" && !specialMk8s {
		// A region has been specified, make sure it exists.
		if _, err := jujucloud.RegionByName(cloud.Regions, cloudRegion); err != nil {
			if cloudRegion == given.CloudRegion {
				// The string is not in the format cloud/region,
				// so we should tell the user that it is
				// neither a cloud nor a region in the
				// controller's cloud.
				finders := []func() (map[names.CloudTag]jujucloud.Cloud, error){}
				if given.LocalOnly {
					finders = append(finders, LocalClouds)
				} else if given.RemoteOnly {
					finders = append(finders, given.RemoteCloudClient.Clouds)
				} else {
					// Both local and remote clouds are needed.
					finders = append(finders, LocalClouds)
					finders = append(finders, given.RemoteCloudClient.Clouds)
				}
				all := map[names.CloudTag]jujucloud.Cloud{}
				for _, f := range finders {
					clouds, err := f()
					if err != nil {
						return fail(errors.Annotate(err, "querying supported clouds"))
					}
					for tag, remoteCloud := range clouds {
						all[tag] = remoteCloud
					}
				}
				return fail(unsupportedCloudOrRegionError(all, given.CloudRegion, given.Command))
			}
			return fail(errors.Trace(err))
		}
	}
	if cloudRegion == "" && given.UseDefaultRegion && !specialMk8s {
		if !remote {
			cred, err := given.LocalStore.CredentialForCloud(cloud.Name)
			if err == nil {
				cloudRegion = cred.DefaultRegion
			}
			if !errors.IsNotFound(err) {
				return fail(err)
			}
		}
		// No default region was found or the cloud is not local.
		if cloudRegion == "" {
			// Get first cloud region.
			if len(cloud.Regions) != 0 {
				cloudRegion = cloud.Regions[0].Name
			}
		}
	}
	return cloudTag, cloud, cloudRegion, nil
}

func LocalClouds() (map[names.CloudTag]jujucloud.Cloud, error) {
	clouds, _, err := jujucloud.PublicCloudMetadata(jujucloud.JujuPublicCloudsPath())
	if err != nil {
		return nil, err
	}
	details := map[names.CloudTag]jujucloud.Cloud{}
	for name, publicCloud := range clouds {
		details[names.NewCloudTag(name)] = publicCloud
	}

	// Add in built in clouds like localhost (lxd).
	builtinClouds, err := BuiltInClouds()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for name, builtInCloud := range builtinClouds {
		details[names.NewCloudTag(name)] = builtInCloud
	}

	personalClouds, err := jujucloud.PersonalCloudMetadata()
	if err != nil {
		return nil, err
	}
	for name, personalCloud := range personalClouds {
		details[names.NewCloudTag(name)] = personalCloud
	}

	return details, nil
}

// MaybeGetControllerCloud tentatively gets a cloud that the controller is bootstrapped to.
func MaybeGetControllerCloud(cloudClient CloudAPI, command string) (names.CloudTag, jujucloud.Cloud, error) {
	clouds, err := cloudClient.Clouds()
	if err != nil {
		return names.CloudTag{}, jujucloud.Cloud{}, errors.Trace(err)
	}
	// TODO (anastasiamac 2019-07-31) This is wrong, we should have a API call
	// that returns controller cloud because in multi-cloud scenario this
	// no longer holds.
	if len(clouds) != 1 {
		return names.CloudTag{}, jujucloud.Cloud{}, unsupportedCloudOrRegionError(clouds, "", command)
	}
	for cloudTag, cloud := range clouds {
		return cloudTag, cloud, nil
	}
	panic("unreachable")
}

func unsupportedCloudOrRegionError(clouds map[names.CloudTag]jujucloud.Cloud, cloudRegion, command string) (err error) {
	cloudNames := make([]string, 0, len(clouds))
	for tag := range clouds {
		cloudNames = append(cloudNames, tag.Id())
	}
	sort.Strings(cloudNames)

	var buf bytes.Buffer
	tw := output.TabWriter(&buf)
	fmt.Fprintln(tw, "Cloud\tRegions")
	for _, cloudName := range cloudNames {
		aCloud := clouds[names.NewCloudTag(cloudName)]
		fmt.Fprintf(tw, "%s\t%s\n", cloudName, strings.Join(jujucloud.RegionNames(aCloud.Regions), ", "))
	}
	tw.Flush()

	var prefix string
	switch len(clouds) {
	case 0:
		return errors.Errorf(`
you do not have %v access to any clouds on this controller.
Please ask the controller administrator to grant you %v permission
for a particular cloud to which you want to add a model.`[1:], command, command)
	case 1:
		for cloudTag := range clouds {
			prefix = fmt.Sprintf(`
%q is neither a cloud supported by this controller,
nor a region in the controller's default cloud %q.`[1:],
				cloudRegion, cloudTag.Id())
		}
	default:
		prefix = `
this controller manages more than one cloud.
Please specify which cloud/region to use:

    juju add-model [options] <model-name> cloud[/region]
`[1:]
	}
	return errors.Errorf("%s\nThe clouds/regions supported by this controller are:\n\n%s", prefix, buf.String())
}

type PersonalCloudMetadataStore interface {
	PersonalCloudMetadata() (map[string]jujucloud.Cloud, error)
	WritePersonalCloudMetadata(cloudsMap map[string]jujucloud.Cloud) error
}

func AddLocalCloud(cloudMetadataStore PersonalCloudMetadataStore, newCloud jujucloud.Cloud) error {
	personalClouds, err := cloudMetadataStore.PersonalCloudMetadata()
	if err != nil {
		return err
	}
	if personalClouds == nil {
		personalClouds = make(map[string]jujucloud.Cloud)
	}
	personalClouds[newCloud.Name] = newCloud
	return cloudMetadataStore.WritePersonalCloudMetadata(personalClouds)
}

func AddLocalCredentials(store jujuclient.ClientStore, cloudName string, credentials jujucloud.CloudCredential) error {
	err := store.UpdateCredential(cloudName, credentials)
	return errors.Trace(err)
}

func AddRemoteCloud(api UploadAPI, newCloud jujucloud.Cloud) error {
	err := api.AddCloud(newCloud)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func VerifyCredentialsForUpload(ctx *cmd.Context, accountDetails *jujuclient.AccountDetails, aCloud *jujucloud.Cloud, region string, all map[string]jujucloud.Credential) (map[string]jujucloud.Credential, error) {
	verified := map[string]jujucloud.Credential{}
	var erred error
	for credentialName, aCredential := range all {
		id := fmt.Sprintf("%s/%s/%s", aCloud.Name, accountDetails.User, credentialName)
		if !names.IsValidCloudCredential(id) {
			ctx.Warningf("Could not update controller credential %v for user %v on cloud %v: %v", credentialName, accountDetails.User, aCloud.Name, errors.NotValidf("cloud credential ID %q", id))
			erred = cmd.ErrSilent
			continue
		}
		verifiedCredential, err := modelcmd.VerifyCredentials(ctx, aCloud, &aCredential, credentialName, region)
		if err != nil {
			logger.Errorf("%v", err)
			ctx.Warningf("Could not verify credential %v for cloud %v locally", credentialName, aCloud.Name)
			erred = cmd.ErrSilent
			continue
		}
		verified[names.NewCloudCredentialTag(id).String()] = *verifiedCredential
	}
	return verified, erred
}
