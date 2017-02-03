package oracle

import (
	"github.com/juju/errors"
	"github.com/juju/jsonschema"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/loggo"

	oci "github.com/hoenirvili/go-oracle-cloud/api"
)

var logger = loggo.GetLogger("juju.provider.oracle")

const (
	providerType = "oracle"
)

// provide friendly aliases for the register provider function
var providerAliases = []string{"ocl", "orcl", "oci"}

// environProvider type implements environs.EnvironProvider interface
// this will represent a computing and storage provider of the orcale cloud
// alongiside environs.EnvironProvider this implements config.Validator interface and
// environs.ProviderCredentials also.
type environProvider struct {
	client *oci.Client
}

// CloudSchema returns the schema used to validate input for add-cloud.  Since
// this provider does support custom clouds, this always returns non-nil
func (e environProvider) CloudSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:     []jsonschema.Type{jsonschema.ObjectType},
		Required: []string{cloud.EndpointKey, cloud.AuthTypesKey, cloud.RegionsKey},
		Order:    []string{cloud.EndpointKey, cloud.AuthTypesKey, cloud.RegionsKey},
		Properties: map[string]*jsonschema.Schema{
			cloud.EndpointKey: {
				Singular: "the API endpoint url for the cloud",
				Type:     []jsonschema.Type{jsonschema.StringType},
				Format:   jsonschema.FormatURI,
			},
			cloud.AuthTypesKey: &jsonschema.Schema{
				// don't need a prompt, since there's only one choice.
				Type: []jsonschema.Type{jsonschema.ArrayType},
				Enum: []interface{}{[]string{string(cloud.UserPassAuthType)}},
			},
			cloud.RegionsKey: {
				Type:     []jsonschema.Type{jsonschema.ObjectType},
				Singular: "region",
				Plural:   "regions",
				AdditionalProperties: &jsonschema.Schema{
					Type:          []jsonschema.Type{jsonschema.ObjectType},
					MaxProperties: jsonschema.Int(0),
				},
			},
		},
	}
}

// Ping tests the connection to the oracle cloud to verify the endoint is valid.
func (e environProvider) Ping(endpoint string) error {
	return nil
}

// PrepareConfig prepares the configuration for the new model, based on
// the provided arguments. PrepareConfig is expected to produce a
// deterministic output
func (e environProvider) PrepareConfig(config environs.PrepareConfigParams) (*config.Config, error) {
	if err := e.checkSpec(config.Cloud); err != nil {
		return nil, errors.Annotatef(err, "validating cloud spec")
	}
	return config.Config, nil
}

// checkSpec will try and see if the config cloud that is generated is on point with the cloudspec.
func (e environProvider) checkSpec(spec environs.CloudSpec) error {
	// also every spec has a internal validate function
	// so we must call it in order to know if everything is ok in this state
	if err := spec.Validate(); err != nil {
		return errors.Trace(err)
	}

	// we must know if the credentials are missing or not
	if spec.Credential == nil {
		return errors.NotValidf("missing credentials")
	}

	// check if the authentication type selected by client match the same auth type
	// that oracle cloud services support.
	if authType := spec.Credential.AuthType(); authType != cloud.UserPassAuthType {
		return errors.NotSupportedf("%q auth-type ", authType)
	}

	return nil
}

// Open opens the oracle environment complaint with Juju and returns it. The configuration must have
// passed through PrepareConfig at some point in its lifecycle.
//
// This operation is not performing any expensive operation.
func (e *environProvider) Open(params environs.OpenParams) (environs.Environ, error) {
	logger.Debugf("opening model %q", params.Config.Name())
	if err := e.checkSpec(params.Cloud); err != nil {
		return nil, errors.Annotatef(err, "validating cloud spec")
	}

	environ := newOracleEnviron(e, params)
	return environ, nil
}

// Validate method will validate model configuration
// This will ensure that the config passed is a valid configuration for the oracle cloud.
// If old is not nil, Validate should use it to determine whether a configuration change is valid.
func (e environProvider) Validate(cfg, old *config.Config) (valid *config.Config, _ error) {
	return nil, nil
}

// CredentialSchemas returns credential schemas, keyed on authentication type. This is used to validate existing oracle credentials, or to generate new ones.
func (e environProvider) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return map[cloud.AuthType]cloud.CredentialSchema{
		cloud.UserPassAuthType: {{
			"username", cloud.CredentialAttr{
				Description: "account username",
			},
		}, {
			"password", cloud.CredentialAttr{
				Description: "account password",
				Hidden:      true,
			},
		}},
	}
}

// DetectCredentials automatically detects one or more oracle credentials from the environmnet. This may involve, for example inspecting environmnet variables, or reading configuration files in well-defined locations.
// If no credentials can be detected, the func will return an error satisfying errors.IsNotFound
func (e environProvider) DetectCredentials() (*cloud.CloudCredential, error) {
	return nil, nil
}

// FinalizeCredential finalizes a oracle credential, updating any attributes as
// necessarry. This is done clinet-side, when adding the credential to credentials.yaml
// and before uploading crdentials to the controller.
// The provider may completely alter a credential, even goiing as far as changing the auth-type, but the output must be a fully formed credential that is orcale complaint.
func (e environProvider) FinalizeCredential(
	cfx environs.FinalizeCredentialContext,
	params environs.FinalizeCredentialParams,
) (*cloud.Credential, error) {
	// we will return the exact credentials that we have entered from the interactive form.
	return &params.Credential, nil
}
