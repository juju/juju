// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package docker

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"

	"github.com/docker/distribution/reference"
	"github.com/juju/errors"
	"github.com/juju/featureflag"
	"github.com/juju/loggo"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/feature"
)

var logger = loggo.GetLogger("juju.docker")

// APIVersion is the API version type.
type APIVersion string

const (
	// APIVersionV1 is the API version v1.
	APIVersionV1 APIVersion = "v1"
	// APIVersionV2 is the API version v2.
	APIVersionV2 APIVersion = "v2"
)

func (v APIVersion) String() string {
	return string(v)
}

// TokenAuthConfig contains authorization information for token auth.
// Juju does not support the docker credential helper because k8s does not support it either.
// https://kubernetes.io/docs/concepts/containers/images/#configuring-nodes-to-authenticate-to-a-private-registry
type TokenAuthConfig struct {
	Email string `json:"email,omitempty" yaml:"email,omitempty"`

	// IdentityToken is used to authenticate the user and get
	// an access token for the registry.
	IdentityToken string `json:"identitytoken,omitempty" yaml:"identitytoken,omitempty"`

	// RegistryToken is a bearer token to be sent to a registry
	RegistryToken string `json:"registrytoken,omitempty" yaml:"registrytoken,omitempty"`
}

// Empty checks if the auth information is empty.
func (ac TokenAuthConfig) Empty() bool {
	return ac.IdentityToken == "" && ac.RegistryToken == ""
}

// Validate validates the spec.
func (ac *TokenAuthConfig) Validate() error {
	return nil
}

func (ac *TokenAuthConfig) init() error {
	return nil
}

// BasicAuthConfig contains authorization information for basic auth.
type BasicAuthConfig struct {
	// Auth is the base64 encoded "username:password" string.
	Auth string `json:"auth,omitempty" yaml:"auth,omitempty"`

	// Username holds the username used to gain access to a non-public image.
	Username string `json:"username" yaml:"username"`

	// Password holds the password used to gain access to a non-public image.
	Password string `json:"password" yaml:"password"`
}

// Empty checks if the auth information is empty.
func (ba BasicAuthConfig) Empty() bool {
	return ba.Auth == "" && ba.Username == "" && ba.Password == ""
}

// Validate validates the spec.
func (ba *BasicAuthConfig) Validate() error {
	return nil
}

func (ba *BasicAuthConfig) init() error {
	if ba.Empty() {
		return nil
	}
	if ba.Auth == "" {
		ba.Auth = base64.StdEncoding.EncodeToString([]byte(ba.Username + ":" + ba.Password))
	}
	return nil
}

// ImageRepoDetails contains authorization information for connecting to a Registry.
type ImageRepoDetails struct {
	BasicAuthConfig `json:",inline" yaml:",inline"`
	TokenAuthConfig `json:",inline" yaml:",inline"`

	// Repository is the namespace of the image repo.
	Repository string `json:"repository,omitempty" yaml:"repository,omitempty"`

	// ServerAddress is the auth server address.
	ServerAddress string `json:"serveraddress,omitempty" yaml:"serveraddress,omitempty"`
}

func (rid ImageRepoDetails) AuthEqual(r ImageRepoDetails) bool {
	return reflect.DeepEqual(rid.BasicAuthConfig, r.BasicAuthConfig) &&
		reflect.DeepEqual(rid.TokenAuthConfig, r.TokenAuthConfig)
}

func (rid ImageRepoDetails) IsPrivate() bool {
	return !rid.BasicAuthConfig.Empty() || !rid.TokenAuthConfig.Empty()
}

type dockerConfigData struct {
	Auths map[string]ImageRepoDetails `json:"auths"`
}

// SecretData returns secret data format.
func (rid ImageRepoDetails) SecretData() ([]byte, error) {
	if rid.BasicAuthConfig.Empty() && rid.TokenAuthConfig.Empty() {
		// No auth information is required for a public repository.
		return nil, nil
	}
	rid.Repository = ""
	o := dockerConfigData{
		Auths: map[string]ImageRepoDetails{
			rid.ServerAddress: rid,
		},
	}
	return json.Marshal(o)
}

// String returns yaml format.
func (rid ImageRepoDetails) String() string {
	d, _ := yaml.Marshal(rid)
	return string(d)
}

// Validate validates the spec.
func (rid *ImageRepoDetails) Validate() error {
	if rid.Repository == "" {
		return errors.NotValidf("empty repository")
	}
	_, err := reference.ParseNormalizedNamed(rid.Repository)
	if err != nil {
		return errors.NewNotValid(err, fmt.Sprintf("docker image path %q", rid.Repository))
	}
	if err := rid.BasicAuthConfig.Validate(); err != nil {
		return errors.Annotatef(err, "validating basic auth config for repository %q", rid.Repository)
	}
	if err := rid.TokenAuthConfig.Validate(); err != nil {
		return errors.Annotatef(err, "validating token auth config for repository %q", rid.Repository)
	}
	return nil
}

func (rid *ImageRepoDetails) init() error {
	if err := rid.BasicAuthConfig.init(); err != nil {
		return errors.Annotatef(err, "initializing basic auth config for repository %q", rid.Repository)
	}
	if err := rid.TokenAuthConfig.init(); err != nil {
		return errors.Annotatef(err, "initializing token auth config for repository %q", rid.Repository)
	}
	return nil
}
func (rid ImageRepoDetails) Empty() bool {
	return rid == ImageRepoDetails{}
}

func (rid ImageRepoDetails) APIVersion() APIVersion {
	if rid.IsPrivate() {
		return APIVersionV2
	}
	return APIVersionV1
}

func fileExists(p string) (bool, error) {
	info, err := os.Stat(p)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, errors.Trace(err)
	}
	return !info.IsDir(), nil
}

// NewImageRepoDetails tries to parse a file path or file content and returns an instance of ImageRepoDetails.
func NewImageRepoDetails(contentOrPath string) (o *ImageRepoDetails, err error) {
	if contentOrPath == "" {
		return o, nil
	}
	data := []byte(contentOrPath)
	isPath, err := fileExists(contentOrPath)
	if err == nil && isPath {
		logger.Debugf("reading image repository information from %q", contentOrPath)
		data, err = ioutil.ReadFile(contentOrPath)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	o = &ImageRepoDetails{}
	err = yaml.Unmarshal(data, o)
	if err != nil {
		return &ImageRepoDetails{Repository: contentOrPath}, nil
	}

	if err = o.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	if err = o.init(); err != nil {
		return nil, errors.Trace(err)
	}
	if o.IsPrivate() && !featureflag.Enabled(feature.PrivateRegistry) {
		return nil, errors.New(
			fmt.Sprintf("private registry support is under development, enable feature flag %q to test it out", feature.PrivateRegistry),
		)
	}
	return o, nil
}
