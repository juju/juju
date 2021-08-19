// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package docker

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"

	"github.com/docker/distribution/reference"
	"github.com/juju/errors"
	"gopkg.in/yaml.v2"
)

// The default server address - https://kubernetes.io/docs/reference/generated/kubectl/kubectl-commands#-em-secret-docker-registry-em-
const defaultServerAddress = "https://index.docker.io/v1/"

// TokenAuthConfig contains authorization information for connecting to a Registry.
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

func (ac TokenAuthConfig) Empty() bool {
	return ac.IdentityToken == "" && ac.RegistryToken == ""
}

func (ac *TokenAuthConfig) Validate() error {
	return nil
}

type BasicAuthConfig struct {
	// Auth is the base64 encoded "username:password" string.
	Auth string `json:"auth,omitempty" yaml:"auth,omitempty"`

	// Username holds the username used to gain access to a non-public image.
	Username string `json:"username,omitempty" yaml:"username,omitempty"`

	// Password holds the password used to gain access to a non-public image.
	Password string `json:"password,omitempty" yaml:"password,omitempty"`
}

func (ba BasicAuthConfig) Empty() bool {
	return ba.Auth == "" && ba.Username == "" && ba.Password == ""
}

func (ba BasicAuthConfig) Validate() error {
	return nil
}

type ImageRepoDetails struct {
	BasicAuthConfig `json:",inline" yaml:",inline"`
	TokenAuthConfig `json:",inline" yaml:",inline"`

	Repository    string `json:"repository,omitempty" yaml:"repository,omitempty"`
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

func (rid *ImageRepoDetails) String() string {
	d, _ := yaml.Marshal(rid)
	return string(d)
}

// NormalizedRepository returns normalized OCI name.
func (rid ImageRepoDetails) RepositoryPath() string {
	named, _ := reference.ParseNormalizedNamed(rid.Repository)
	return reference.Path(named)
}

func (rid ImageRepoDetails) Validate() error {
	if rid.Repository == "" {
		return errors.NotValidf("empty repository")
	}
	_, err := reference.ParseNormalizedNamed(rid.Repository)
	if err != nil {
		return errors.NewNotValid(err, fmt.Sprintf("docker image path %q", rid.Repository))
	}
	if rid.BasicAuthConfig.Validate(); err != nil {
		return errors.Trace(err)
	}
	if rid.TokenAuthConfig.Validate(); err != nil {
		return errors.Trace(err)
	}
	return nil
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

func NewImageRepoDetails(contentOrPath string) (*ImageRepoDetails, error) {
	data := []byte(contentOrPath)
	isPath, err := fileExists(contentOrPath)
	if err == nil && isPath {
		data, err = ioutil.ReadFile(contentOrPath)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	o := &ImageRepoDetails{}
	err = yaml.Unmarshal([]byte(data), o)
	if err != nil {
		return &ImageRepoDetails{Repository: contentOrPath}, nil
	}
	if o.ServerAddress == "" {
		o.ServerAddress = defaultServerAddress
	}
	return o, nil
}
