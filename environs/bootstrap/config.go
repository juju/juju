// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/errors"
	"github.com/juju/schema"
	"github.com/juju/utils"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/pki"
)

const (
	// AdminSecretKey is the attribute key for the administrator password.
	AdminSecretKey = "admin-secret"

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
// config.
var BootstrapConfigAttributes = []string{
	AdminSecretKey,
	CACertKey,
	CAPrivateKeyKey,
	BootstrapTimeoutKey,
	BootstrapRetryDelayKey,
	BootstrapAddressesDelayKey,
	ControllerServiceType,
	ControllerExternalName,
	ControllerExternalIPs,
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
	AdminSecret             string
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
	coerced, err := configChecker.Coerce(attrs, nil)
	if err != nil {
		return Config{}, errors.Trace(err)
	}
	attrs = coerced.(map[string]interface{})
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
	data, err := ioutil.ReadFile(absPath)
	if err != nil {
		return "", userSpecified, errors.Annotatef(err, "%q not set, and could not read from %q", key, path)
	}
	if len(data) == 0 {
		return "", userSpecified, errors.Errorf("file %q is empty", path)
	}
	return string(data), userSpecified, nil
}

var configChecker = schema.FieldMap(schema.Fields{
	AdminSecretKey:            schema.String(),
	CACertKey:                 schema.String(),
	CACertKey + "-path":       schema.String(),
	CAPrivateKeyKey:           schema.String(),
	CAPrivateKeyKey + "-path": schema.String(),
	ControllerServiceType: schema.OneOf(
		schema.Const(string(caas.ServiceCluster)),
		schema.Const(string(caas.ServiceLoadBalancer)),
		schema.Const(string(caas.ServiceExternal)),
	),
	ControllerExternalName:     schema.String(),
	ControllerExternalIPs:      schema.List(schema.String()),
	BootstrapTimeoutKey:        schema.ForceInt(),
	BootstrapRetryDelayKey:     schema.ForceInt(),
	BootstrapAddressesDelayKey: schema.ForceInt(),
}, schema.Defaults{
	AdminSecretKey:             schema.Omit,
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
})
