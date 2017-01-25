package oracle

import (
	"os"

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

func (e environProvider) CloudSchema() *jsonschema.Schema {
	return nil
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
	os.Exit(1)
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
func (e environProvider) Open(params environs.OpenParams) (environs.Environ, error) {
	logger.Debugf("opening model %q", params.Config.Name())
	return nil, nil
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
