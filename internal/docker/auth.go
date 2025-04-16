// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package docker

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/distribution/reference"
	"github.com/juju/errors"

	internallogger "github.com/juju/juju/internal/logger"
)

var logger = internallogger.GetLogger("juju.docker")

// Token defines a token value with expiration time.
type Token struct {
	// Value is the value of the token.
	Value string

	// ExpiresAt is the unix time in seconds and milliseconds when the authorization token expires.
	ExpiresAt *time.Time
}

// UnmarshalJSON implements the json.Unmarshaller interface.
func (t *Token) UnmarshalJSON(value []byte) error {
	err := json.Unmarshal(value, &t.Value)
	return errors.Trace(err)
}

// String returns the string value.
func (t *Token) String() string {
	if t.Empty() {
		return ""
	}
	o := NewToken(t.Value)
	o.mask()
	return o.Value
}

// Content returns the raw content of the token.
func (t *Token) Content() string {
	if t.Empty() {
		return ""
	}
	return t.Value
}

// MarshalJSON implements the json.Marshaller interface.
func (t Token) MarshalJSON() ([]byte, error) {
	if t.Empty() {
		return nil, nil
	}
	return json.Marshal(t.Value)
}

// NewToken creates a Token.
func NewToken(value string) *Token {
	if value == "" {
		return nil
	}
	return &Token{Value: value}
}

// Empty checks if the auth information is empty.
func (t *Token) Empty() bool {
	return t == nil || t.Value == ""
}

// Mask hides the token value.
func (t *Token) mask() {
	if t.Empty() {
		return
	}
	var b bytes.Buffer
	for range t.Value {
		b.WriteString("*")
	}
	t.Value = b.String()
}

// TokenAuthConfig contains authorization information for token auth.
// Juju does not support the docker credential helper because k8s does not support it either.
// https://kubernetes.io/docs/concepts/containers/images/#configuring-nodes-to-authenticate-to-a-private-registry
type TokenAuthConfig struct {
	Email string `json:"email,omitempty" yaml:"email,omitempty"`

	// IdentityToken is used to authenticate the user and get
	// an access token for the registry.
	IdentityToken *Token `json:"identitytoken,omitempty" yaml:"identitytoken,omitempty"`

	// RegistryToken is a bearer token to be sent to a registry
	RegistryToken *Token `json:"registrytoken,omitempty" yaml:"registrytoken,omitempty"`
}

// Empty checks if the auth information is empty.
func (ac TokenAuthConfig) Empty() bool {
	return ac.RegistryToken.Empty() && ac.IdentityToken.Empty()
}

// Validate validates the spec.
func (ac *TokenAuthConfig) Validate() error {
	return nil
}

// BasicAuthConfig contains authorization information for basic auth.
type BasicAuthConfig struct {
	// Auth is the base64 encoded "username:password" string.
	Auth *Token `json:"auth,omitempty" yaml:"auth,omitempty"`

	// Username holds the username used to gain access to a non-public image.
	Username string `json:"username" yaml:"username"`

	// Password holds the password used to gain access to a non-public image.
	Password string `json:"password" yaml:"password"`
}

// Empty checks if the auth information is empty.
func (ba BasicAuthConfig) Empty() bool {
	return ba.Auth.Empty() && ba.Username == "" && ba.Password == ""
}

// Validate validates the spec.
func (ba *BasicAuthConfig) Validate() error {
	return nil
}

// DockerImageDetails holds the details for a Docker resource type.
type DockerImageDetails struct {
	// RegistryPath holds the path of the Docker image (including host and sha256) in a docker registry.
	RegistryPath string `json:"ImageName" yaml:"registrypath"`

	ImageRepoDetails `json:",inline" yaml:",inline"`
}

// ImageRepoDetails contains authorization information for connecting to a Registry.
type ImageRepoDetails struct {
	BasicAuthConfig `json:",inline" yaml:",inline"`
	TokenAuthConfig `json:",inline" yaml:",inline"`

	// Repository is the namespace of the image repo.
	Repository string `json:"repository,omitempty" yaml:"repository,omitempty"`

	// ServerAddress is the auth server address.
	ServerAddress string `json:"serveraddress,omitempty" yaml:"serveraddress,omitempty"`

	// Region is the cloud region.
	Region string `json:"region,omitempty" yaml:"region,omitempty"`
}

// AuthEqual compares if the provided one equals to current repository detail.
func (rid ImageRepoDetails) AuthEqual(r ImageRepoDetails) bool {
	return reflect.DeepEqual(rid.BasicAuthConfig, r.BasicAuthConfig) &&
		reflect.DeepEqual(rid.TokenAuthConfig, r.TokenAuthConfig)
}

// IsPrivate checks if the repository detail is private.
func (rid ImageRepoDetails) IsPrivate() bool {
	return !rid.BasicAuthConfig.Empty() || !rid.TokenAuthConfig.Empty()
}

type dockerConfigData struct {
	Auths map[string]ImageRepoDetails `json:"auths"`
}

// SecretData returns secret data format.
func (rid ImageRepoDetails) SecretData() ([]byte, error) {
	if rid.BasicAuthConfig.Empty() && rid.TokenAuthConfig.Empty() {
		return nil, nil
	}
	repo := strings.Split(rid.Repository, "/")[0]
	rid.Repository = ""
	if !rid.BasicAuthConfig.Empty() && rid.BasicAuthConfig.Auth.Empty() {
		rid.BasicAuthConfig.Auth = NewToken(
			base64.StdEncoding.EncodeToString([]byte(rid.BasicAuthConfig.Username + ":" + rid.BasicAuthConfig.Password)))
	}
	o := dockerConfigData{
		Auths: map[string]ImageRepoDetails{
			repo: rid,
		},
	}
	return json.Marshal(o)
}

// Content returns the json marshalled string with raw credentials.
func (rid ImageRepoDetails) Content() string {
	copy := rid
	copy.Repository = ""
	if copy.Empty() {
		// If only repository is set, return it.
		return rid.Repository
	}
	d, _ := json.Marshal(rid)
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

// Empty checks if the auth information is empty.
func (rid ImageRepoDetails) Empty() bool {
	return rid == ImageRepoDetails{}
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

// NewImageRepoDetails tries to parse as json or basic repository path and returns an instance of ImageRepoDetails.
func NewImageRepoDetails(repo string) (o ImageRepoDetails, err error) {
	if repo == "" {
		return o, nil
	}
	data := []byte(repo)
	err = json.Unmarshal(data, &o)
	if err != nil {
		logger.Tracef(context.TODO(), "unmarshalling %q, err %#v", repo, err)
		return ImageRepoDetails{Repository: repo}, nil
	}
	if err = o.Validate(); err != nil {
		return o, errors.Trace(err)
	}
	return o, nil
}

// LoadImageRepoDetails tries to parse a file path or file content and returns an instance of ImageRepoDetails.
func LoadImageRepoDetails(contentOrPath string) (o ImageRepoDetails, err error) {
	if contentOrPath == "" {
		return o, nil
	}
	data := []byte(contentOrPath)
	isPath, err := fileExists(contentOrPath)
	if err == nil && isPath {
		logger.Debugf(context.TODO(), "reading image repository information from %q", contentOrPath)
		data, err = os.ReadFile(contentOrPath)
		if err != nil {
			return o, errors.Trace(err)
		}
	}
	return NewImageRepoDetails(string(data))
}
