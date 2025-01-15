// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/errors"
	"github.com/juju/schema"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/caas"
	coreconfig "github.com/juju/juju/core/config"
	"github.com/juju/juju/internal/configschema"
	"github.com/juju/juju/internal/pki"
	"github.com/juju/juju/internal/ssh"
	"github.com/juju/juju/juju/osenv"
)

const (
	// AdminSecretKey is the attribute key for the administrator password.
	AdminSecretKey = "admin-secret"

	// authorizedKeysDelimiter denotes the delimiter used when reading the value
	// for [AuthorizedKeysKey] to separate multiple ssh public keys.
	authorizedKeysDelimiter = ';'

	// AuthorizedKeysKey is the key used for supplying additional authorized
	// keys to be allowed to the controller model during bootstrap.
	AuthorizedKeysKey = "authorized-keys"

	// AuthorizedKeysPathKey is the key used for supplying a path to an
	// authorized key file that will be used for adding additional authorized
	// keys to the controller model during bootstrap.
	AuthorizedKeysPathKey = AuthorizedKeysKey + "-path"

	// CACertKey is the attribute key for the controller's CA certificate.
	CACertKey = "ca-cert"

	// CAPrivateKeyKey is the key for the controller's CA certificate private key.
	CAPrivateKeyKey = "ca-private-key"

	// BootstrapTimeoutKey is the attribute key for the amount of time to wait
	// for bootstrap to complete.
	BootstrapTimeoutKey = "bootstrap-timeout"

	// BootstrapRetryDelayKey is the attribute key for the amount of time
	// in between attempts to connect to a bootstrap machine address.
	BootstrapRetryDelayKey = "bootstrap-retry-delay"

	// BootstrapAddressesDelayKey is the attribute key for the amount of
	// time in between refreshing the bootstrap machine addresses.
	BootstrapAddressesDelayKey = "bootstrap-addresses-delay"

	// ControllerServiceType is for k8s controllers to override
	// the opinionated service type for a given cluster.
	ControllerServiceType = "controller-service-type"

	// ControllerExternalName sets the external name
	// for a k8s controller of type external.
	ControllerExternalName = "controller-external-name"

	// ControllerExternalIPs is used to specify a comma separated
	// list of external IPs for a k8s controller of type external.
	ControllerExternalIPs = "controller-external-ips"
)

const (
	// Attribute Defaults

	// DefaultBootstrapSSHTimeout is the amount of time to wait
	// contacting a controller, in seconds.
	DefaultBootstrapSSHTimeout = 1200

	// DefaultBootstrapSSHRetryDelay is the amount of time between
	// attempts to connect to an address, in seconds.
	DefaultBootstrapSSHRetryDelay = 5

	// DefaultBootstrapSSHAddressesDelay is the amount of time betwee
	// refreshing the addresses, in seconds. Not too frequent, as we
	// refresh addresses from the provider each time.
	DefaultBootstrapSSHAddressesDelay = 10
)

// BootstrapConfigAttributes are attributes which may be defined by the
// user at bootstrap time, but should not be present in general controller
// config or model config.
var BootstrapConfigAttributes = []string{
	AdminSecretKey,
	AuthorizedKeysKey,
	AuthorizedKeysPathKey,
	CACertKey,
	CAPrivateKeyKey,
	BootstrapTimeoutKey,
	BootstrapRetryDelayKey,
	BootstrapAddressesDelayKey,
	ControllerServiceType,
	ControllerExternalName,
	ControllerExternalIPs,
}

// BootstrapConfigSchema returns the schema used for config items during
// bootstrap.
func BootstrapConfigSchema() configschema.Fields {
	return configschema.Fields{
		// TODO (tlm): It is unclear why we define this schema twice in this file.
		// Take a look at [configSchema] that repeats this information again and is
		// what is actually used by this file. This information is purely used for
		// display purposed in help for the bootstrap command.
		//
		// Ideally we can just merge these two schemas together and stop repeating
		// ourselves.
		AdminSecretKey: {
			Description: "Sets the Juju administrator password",
			Type:        configschema.Tstring,
		},
		AuthorizedKeysKey: {
			Description: "Additional authorized SSH public keys for the " +
				"initial controller model, as found in a " +
				"~/.ssh/authorized_keys file. Multiple keys are delimited by ';'",
			Type: configschema.Tstring,
		},
		AuthorizedKeysPathKey: {
			Description: fmt.Sprintf(
				"Additional authorized SSH public keys to be read "+
					"from a ~/.ssh/authorized_keys file. Keys defined in this "+
					"file are appended to those already defined in %s",
				AuthorizedKeysKey,
			),
			Type: configschema.Tstring,
		},
		CACertKey: {
			Description: fmt.Sprintf(
				"Sets the bootstrapped controllers CA cert to use and issue "+
					"certificates from, used in conjunction with %s",
				CAPrivateKeyKey),
			Type: configschema.Tstring,
		},
		CAPrivateKeyKey: {
			Description: fmt.Sprintf(
				"Sets the bootstrapped controllers CA cert private key to sign "+
					"certificates with, used in conjunction with %s",
				CACertKey),
			Type: configschema.Tstring,
		},
		BootstrapTimeoutKey: {
			Description: "Controls how long Juju will wait for a bootstrap to " +
				"complete before considering it failed in seconds",
			Type: configschema.Tint,
		},
		BootstrapRetryDelayKey: {
			Description: "Controls the amount of time in seconds between attempts " +
				"to connect to a bootstrap machine address",
			Type: configschema.Tint,
		},
		BootstrapAddressesDelayKey: {
			Description: "Controls the amount of time in seconds in between " +
				"refreshing the bootstrap machine addresses",
			Type: configschema.Tint,
		},
		ControllerServiceType: {
			Description: "Controls the kubernetes service type for Juju " +
				"controllers, see\n" +
				"https://kubernetes.io/docs/reference/kubernetes-api/service-resources/service-v1/#ServiceSpec\n" +
				"valid values are one of cluster, loadbalancer, external",
			Type: configschema.Tstring,
		},
		ControllerExternalName: {
			Description: "Sets the external name for a k8s controller of type " +
				"external",
			Type: configschema.Tstring,
		},
		ControllerExternalIPs: {
			Description: "Specifies a comma separated list of external IPs for a " +
				"k8s controller of type external",
			Type: configschema.Tlist,
		},
	}
}

// IsBootstrapAttribute reports whether or not the specified
// attribute name is only relevant during bootstrap.
func IsBootstrapAttribute(attr string) bool {
	for _, a := range BootstrapConfigAttributes {
		if attr == a {
			return true
		}
	}
	return false
}

// Config contains bootstrap-specific configuration.
type Config struct {
	AdminSecret string
	// AuthorizedKeys is a set of additional authorized keys to be used during
	// bootstrap.
	AuthorizedKeys          []string
	CACert                  string
	CAPrivateKey            string
	ControllerServiceType   string
	ControllerExternalName  string
	ControllerExternalIPs   []string
	BootstrapTimeout        time.Duration
	BootstrapRetryDelay     time.Duration
	BootstrapAddressesDelay time.Duration
}

// Validate validates the controller configuration.
func (c Config) Validate() error {
	if c.AdminSecret == "" {
		return errors.NotValidf("empty " + AdminSecretKey)
	}
	if _, err := tls.X509KeyPair([]byte(c.CACert), []byte(c.CAPrivateKey)); err != nil {
		return errors.Annotatef(err, "validating %s and %s", CACertKey, CAPrivateKeyKey)
	}
	if c.BootstrapTimeout <= 0 {
		return errors.NotValidf("%s of %s", BootstrapTimeoutKey, c.BootstrapTimeout)
	}
	if c.BootstrapRetryDelay <= 0 {
		return errors.NotValidf("%s of %s", BootstrapRetryDelayKey, c.BootstrapRetryDelay)
	}
	if c.BootstrapAddressesDelay <= 0 {
		return errors.NotValidf("%s of %s", BootstrapAddressesDelayKey, c.BootstrapAddressesDelay)
	}
	if len(c.ControllerExternalIPs) > 0 &&
		c.ControllerServiceType != string(caas.ServiceExternal) &&
		c.ControllerServiceType != string(caas.ServiceLoadBalancer) {
		return errors.NewNotValid(nil, fmt.Sprintf("external IPs require a service type of %q or %q", caas.ServiceExternal, caas.ServiceLoadBalancer))
	}
	if len(c.ControllerExternalIPs) > 1 && c.ControllerServiceType == string(caas.ServiceLoadBalancer) {
		return errors.NewNotValid(nil, fmt.Sprintf("only 1 external IP is allowed with service type %q", caas.ServiceLoadBalancer))
	}
	return nil
}

// NewConfig creates a new Config from the supplied attributes.
// Default values will be used where defaults are available.
//
// If ca-cert or ca-private-key are not set, then we will check
// if ca-cert-path or ca-private-key-path are set, and read the
// contents. If none of those are set, we will look for files
// in well-defined locations: $JUJU_DATA/ca-cert.pem, and
// $JUJU_DATA/ca-private-key.pem. If none of these are set, an
// error is returned.
func NewConfig(attrs map[string]interface{}) (Config, error) {
	cfg, err := coreconfig.NewConfig(attrs, configSchema, configDefaults)
	if err != nil {
		return Config{}, errors.Trace(err)
	}
	attrs = cfg.Attributes()
	config := Config{
		BootstrapTimeout:        time.Duration(attrs[BootstrapTimeoutKey].(int)) * time.Second,
		BootstrapRetryDelay:     time.Duration(attrs[BootstrapRetryDelayKey].(int)) * time.Second,
		BootstrapAddressesDelay: time.Duration(attrs[BootstrapAddressesDelayKey].(int)) * time.Second,
	}
	if controllerServiceType, ok := attrs[ControllerServiceType].(string); ok {
		config.ControllerServiceType = controllerServiceType
	}
	if controllerExternalName, ok := attrs[ControllerExternalName].(string); ok {
		config.ControllerExternalName = controllerExternalName
	}
	if externalIps, ok := attrs[ControllerExternalIPs].([]interface{}); ok {
		for _, ip := range externalIps {
			config.ControllerExternalIPs = append(config.ControllerExternalIPs, ip.(string))
		}
	}
	if adminSecret, ok := attrs[AdminSecretKey].(string); ok {
		config.AdminSecret = adminSecret
	} else {
		// Generate a random admin secret.
		buf := make([]byte, 16)
		if _, err := io.ReadFull(rand.Reader, buf); err != nil {
			return Config{}, errors.Annotate(err, "generating random "+AdminSecretKey)
		}
		config.AdminSecret = fmt.Sprintf("%x", buf)
	}

	if caCert, ok := attrs[CACertKey].(string); ok {
		config.CACert = caCert
	} else {
		var userSpecified bool
		var err error
		config.CACert, userSpecified, err = readFileAttr(attrs, CACertKey, CACertKey+".pem")
		if err != nil && (userSpecified || !os.IsNotExist(errors.Cause(err))) {
			return Config{}, errors.Annotatef(err, "reading %q from file", CACertKey)
		}
	}

	if caPrivateKey, ok := attrs[CAPrivateKeyKey].(string); ok {
		config.CAPrivateKey = caPrivateKey
	} else {
		var userSpecified bool
		var err error
		config.CAPrivateKey, userSpecified, err = readFileAttr(attrs, CAPrivateKeyKey, CAPrivateKeyKey+".pem")
		if err != nil && (userSpecified || !os.IsNotExist(errors.Cause(err))) {
			return Config{}, errors.Annotatef(err, "reading %q from file", CAPrivateKeyKey)
		}
	}

	if config.CACert == "" && config.CAPrivateKey == "" {
		// Generate a new CA certificate and private key.
		// TODO(perrito666) 2016-05-02 lp:1558657
		signer, err := pki.DefaultKeyProfile()
		if err != nil {
			return Config{}, errors.Annotate(err, "generating new CA key pair")
		}

		ca, err := pki.NewCA("juju-ca", signer)
		if err != nil {
			return Config{}, errors.Annotate(err, "generating new CA")
		}

		caKeyPem, err := pki.SignerToPemString(signer)
		if err != nil {
			return Config{}, errors.Annotate(err, "converting private key to pem")
		}

		caCertPem, err := pki.CertificateToPemString(pki.DefaultPemHeaders, ca)
		if err != nil {
			return Config{}, errors.Annotate(err, "converting certificate to pem")
		}

		config.CACert = caCertPem
		config.CAPrivateKey = caKeyPem
	}

	// If authorized keys is not returned we will just get back the zero value
	// of a string which is safe to parse.
	authorizedKeys, _ := attrs[AuthorizedKeysKey].(string)
	config.AuthorizedKeys, err = ssh.SplitAuthorizedKeysByDelimiter(authorizedKeysDelimiter, authorizedKeys)
	if err != nil {
		return Config{}, fmt.Errorf("cannot parse and split authorized keys: %w", err)
	}

	if authorizedKeysFilePath, ok := attrs[AuthorizedKeysPathKey].(string); ok {
		file, err := os.Open(authorizedKeysFilePath)
		if err != nil {
			return Config{}, fmt.Errorf(
				"cannot open authorised key file %q: %w",
				authorizedKeysFilePath, err,
			)
		}
		defer file.Close()

		keys, err := ssh.SplitAuthorizedKeysReader(file)
		if err != nil {
			return Config{}, fmt.Errorf(
				"cannot split authorized key file %q: %w",
				authorizedKeysFilePath, err,
			)
		}

		config.AuthorizedKeys = append(config.AuthorizedKeys, keys...)
	}

	return config, config.Validate()
}

// readFileAttr reads the contents of an attribute from a file, if the
// corresponding "-path" attribute is set, or otherwise from a default
// path.
func readFileAttr(attrs map[string]interface{}, key, defaultPath string) (content string, userSpecified bool, _ error) {
	path, ok := attrs[key+"-path"].(string)
	if ok {
		userSpecified = true
	} else {
		path = defaultPath
	}
	absPath, err := utils.NormalizePath(path)
	if err != nil {
		return "", userSpecified, errors.Trace(err)
	}
	if !filepath.IsAbs(absPath) {
		absPath = osenv.JujuXDGDataHomePath(absPath)
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", userSpecified, errors.Annotatef(err, "%q not set, and could not read from %q", key, path)
	}
	if len(data) == 0 {
		return "", userSpecified, errors.Errorf("file %q is empty", path)
	}
	return string(data), userSpecified, nil
}

var configSchema = configschema.Fields{
	AdminSecretKey: {
		Type:  configschema.Tstring,
		Group: configschema.JujuGroup,
	},
	AuthorizedKeysKey: {
		Type:  configschema.Tstring,
		Group: configschema.JujuGroup,
	},
	AuthorizedKeysPathKey: {
		Type:  configschema.Tstring,
		Group: configschema.JujuGroup,
	},
	CACertKey: {
		Type:  configschema.Tstring,
		Group: configschema.JujuGroup,
	},
	CACertKey + "-path": {
		Type:  configschema.Tstring,
		Group: configschema.JujuGroup,
	},
	CAPrivateKeyKey: {
		Type:  configschema.Tstring,
		Group: configschema.JujuGroup,
	},
	CAPrivateKeyKey + "-path": {
		Type:  configschema.Tstring,
		Group: configschema.JujuGroup,
	},
	ControllerExternalName: {
		Type:  configschema.Tstring,
		Group: configschema.JujuGroup,
	},
	ControllerExternalIPs: {
		Type:  configschema.Tlist,
		Group: configschema.JujuGroup,
	},
	ControllerServiceType: {
		Type:  configschema.Tstring,
		Group: configschema.JujuGroup,
		Values: []interface{}{
			string(caas.ServiceCluster),
			string(caas.ServiceLoadBalancer),
			string(caas.ServiceExternal),
		},
	},
	BootstrapTimeoutKey: {
		Type:  configschema.Tint,
		Group: configschema.JujuGroup,
	},
	BootstrapRetryDelayKey: {
		Type:  configschema.Tint,
		Group: configschema.JujuGroup,
	},
	BootstrapAddressesDelayKey: {
		Type:  configschema.Tint,
		Group: configschema.JujuGroup,
	},
}

var configDefaults = schema.Defaults{
	AdminSecretKey:             schema.Omit,
	AuthorizedKeysKey:          schema.Omit,
	AuthorizedKeysPathKey:      schema.Omit,
	CACertKey:                  schema.Omit,
	CACertKey + "-path":        schema.Omit,
	CAPrivateKeyKey:            schema.Omit,
	CAPrivateKeyKey + "-path":  schema.Omit,
	ControllerServiceType:      schema.Omit,
	ControllerExternalName:     schema.Omit,
	ControllerExternalIPs:      schema.Omit,
	BootstrapTimeoutKey:        DefaultBootstrapSSHTimeout,
	BootstrapRetryDelayKey:     DefaultBootstrapSSHRetryDelay,
	BootstrapAddressesDelayKey: DefaultBootstrapSSHAddressesDelay,
}
