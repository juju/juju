package packet

import (
	"fmt"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/juju/errors"
	"github.com/juju/jsonschema"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/lxc/lxd/shared/logger"
	"github.com/packethost/packngo"
)

type environProvider struct {
	environProviderCredentials
}

func (p environProvider) CloudSchema() *jsonschema.Schema {
	trace()
	return nil
}

// Ping tests the connection to the cloud, to verify the endpoint is valid.
func (p environProvider) Ping(ctx context.ProviderCallContext, endpoint string) error {
	trace()
	return errors.NotImplementedf("Ping")
}

// PrepareConfig is part of the EnvironProvider interface.
func (p environProvider) PrepareConfig(args environs.PrepareConfigParams) (*config.Config, error) {
	trace()
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	// Set the default block-storage source.
	attrs := make(map[string]interface{})
	// if _, ok := args.Config.StorageDefaultBlockSource(); !ok {
	// 	attrs[config.StorageDefaultBlockSourceKey] = azureStorageProviderType
	// }
	// fmt.Println("config attrs", args.Config)

	if len(attrs) == 0 {
		return args.Config, nil
	}
	return args.Config, nil
}

func validateCloudSpec(spec environscloudspec.CloudSpec) error {
	trace()
	// if err := spec.Validate(); err != nil {
	// 	return errors.Trace(err)
	// }
	// if spec.Credential == nil {
	// 	return errors.NotValidf("missing credential")
	// }
	// if authType := spec.Credential.AuthType(); authType != clientCredentialsAuthType {
	// 	return errors.NotSupportedf("%q auth-type", authType)
	// }
	return nil
}

// Open is specified in the EnvironProvider interface.
func (p environProvider) Open(args environs.OpenParams) (environs.Environ, error) {
	trace()
	logger.Debugf("opening model %q", args.Config.Name())

	e := new(environ)
	e.name = args.Config.Name()

	if err := e.SetCloudSpec(args.Cloud); err != nil {
		return nil, err
	}

	if err := e.SetConfig(args.Config); err != nil {
		return nil, errors.Trace(err)
	}
	return e, nil
}

func (environProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	trace()
	newEcfg, err := validateConfig(cfg, old)
	if err != nil {
		return nil, fmt.Errorf("invalid Packet provider config: %v", err)
	}
	return newEcfg.config.Apply(newEcfg.attrs)
}

func (p environProvider) newConfig(cfg *config.Config) (*environConfig, error) {
	trace()
	valid, err := p.Validate(cfg, nil)
	if err != nil {
		return nil, err
	}
	return &environConfig{valid, valid.UnknownAttrs()}, nil
}

func (e *environ) SetCloudSpec(spec environscloudspec.CloudSpec) error {
	trace()
	e.ecfgMutex.Lock()
	defer e.ecfgMutex.Unlock()

	e.cloud = spec

	// var err error
	e.packetClient = packetClient(e.cloud)

	return nil
}

func packetClient(spec environscloudspec.CloudSpec) *packngo.Client {
	trace()
	credentialAttrs := spec.Credential.Attributes()

	apiToken := credentialAttrs["api-token"]
	httpClient := retryablehttp.NewClient()
	httpClient.RetryWaitMin = time.Second
	httpClient.RetryWaitMax = 30 * time.Second
	httpClient.RetryMax = 10
	// httpClient.CheckRetry = PacketRetryPolicy
	// httpClient.HTTPClient.Transport = logging.NewTransport(
	// 	"Equinix Metal",
	// 	httpClient.HTTPClient.Transport)

	c := packngo.NewClientWithAuth("juju", apiToken, httpClient)

	return c
}

func (environProvider) Version() int {
	trace()
	return 0
}
