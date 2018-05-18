// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package cloud provides functionality to parse information
// describing clouds, including regions, supported auth types etc.
package cloud

import (
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/juju/osenv"
)

//go:generate go run ../generate/filetoconst/filetoconst.go fallbackPublicCloudInfo fallback-public-cloud.yaml fallback_public_cloud.go 2015 cloud

// AuthType is the type of authentication used by the cloud.
type AuthType string

// AuthTypes is defined to allow sorting AuthType slices.
type AuthTypes []AuthType

func (a AuthTypes) Len() int           { return len(a) }
func (a AuthTypes) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a AuthTypes) Less(i, j int) bool { return a[i] < a[j] }

const (
	// AccessKeyAuthType is an authentication type using a key and secret.
	AccessKeyAuthType AuthType = "access-key"

	// UserPassAuthType is an authentication type using a username and password.
	UserPassAuthType AuthType = "userpass"

	// UserPassAuthType is an authentication type using a username and password and a client certificate
	UserPassWithCertAuthType AuthType = "userpasswithcert"

	// OAuth1AuthType is an authentication type using oauth1.
	OAuth1AuthType AuthType = "oauth1"

	// OAuth2AuthType is an authentication type using oauth2.
	OAuth2AuthType AuthType = "oauth2"

	// OAuth2WithCertAuthType is an authentication type using oauth2 and a client certificate
	OAuth2WithCertAuthType AuthType = "oauth2withcert"

	// JSONFileAuthType is an authentication type that takes a path to
	// a JSON file.
	JSONFileAuthType AuthType = "jsonfile"

	// CertificateAuthType is an authentication type using certificates.
	CertificateAuthType AuthType = "certificate"

	// HTTPSigAuthType is an authentication type that uses HTTP signatures:
	// https://tools.ietf.org/html/draft-cavage-http-signatures-06
	HTTPSigAuthType AuthType = "httpsig"

	// EmptyAuthType is the authentication type used for providers
	// that require no credentials, e.g. "lxd", and "manual".
	EmptyAuthType AuthType = "empty"

	// AuthTypesKey is the name of the key in a cloud config or cloud schema
	// that holds the cloud's auth types.
	AuthTypesKey = "auth-types"

	// EndpointKey is the name of the key in a cloud config or cloud schema
	// that holds the cloud's endpoint url.
	EndpointKey = "endpoint"

	// RegionsKey is the name of the key in a cloud schema that holds the list
	// of regions a cloud supports.
	RegionsKey = "regions"
)

// Attrs serves as a map to hold regions specific configuration attributes.
// This serves to reduce confusion over having a nested map, i.e.
// map[string]map[string]interface{}
type Attrs map[string]interface{}

// RegionConfig holds a map of regions and the attributes that serve as the
// region specific configuration options. This allows model inheritance to
// function, providing a place to store configuration for a specific region
// which is  passed down to other models under the same controller.
type RegionConfig map[string]Attrs

// Cloud is a cloud definition.
type Cloud struct {
	// Name of the cloud.
	Name string

	// Type is the type of cloud, eg ec2, openstack etc.
	// This is one of the provider names registered with
	// environs.RegisterProvider.
	Type string

	// Description describes the type of cloud.
	Description string

	// AuthTypes are the authentication modes supported by the cloud.
	AuthTypes AuthTypes

	// Endpoint is the default endpoint for the cloud regions, may be
	// overridden by a region.
	Endpoint string

	// IdentityEndpoint is the default identity endpoint for the cloud
	// regions, may be overridden by a region.
	IdentityEndpoint string

	// StorageEndpoint is the default storage endpoint for the cloud
	// regions, may be overridden by a region.
	StorageEndpoint string

	// Regions are the regions available in the cloud.
	//
	// Regions is a slice, and not a map, because order is important.
	// The first region in the slice is the default region for the
	// cloud.
	Regions []Region

	// Config contains optional cloud-specific configuration to use
	// when bootstrapping Juju in this cloud. The cloud configuration
	// will be combined with Juju-generated, and user-supplied values;
	// user-supplied values taking precedence.
	Config map[string]interface{}

	// RegionConfig contains optional region specific configuration.
	// Like Config above, this will be combined with Juju-generated and user
	// supplied values; with user supplied values taking precedence.
	RegionConfig RegionConfig

	// CACertificates contains an optional list of Certificate
	// Authority certificates to be used to validate certificates
	// of cloud infrastructure components
	// The contents are Base64 encoded x.509 certs.
	CACertificates []string
}

// Region is a cloud region.
type Region struct {
	// Name is the name of the region.
	Name string

	// Endpoint is the region's primary endpoint URL.
	Endpoint string

	// IdentityEndpoint is the region's identity endpoint URL.
	// If the cloud/region does not have an identity-specific
	// endpoint URL, this will be empty.
	IdentityEndpoint string

	// StorageEndpoint is the region's storage endpoint URL.
	// If the cloud/region does not have a storage-specific
	// endpoint URL, this will be empty.
	StorageEndpoint string
}

// cloudSet contains cloud definitions, used for marshalling and
// unmarshalling.
type cloudSet struct {
	// Clouds is a map of cloud definitions, keyed on cloud name.
	Clouds map[string]*cloud `yaml:"clouds"`
}

// cloud is equivalent to Cloud, for marshalling and unmarshalling.
type cloud struct {
	Name             string                 `yaml:"name,omitempty"`
	Type             string                 `yaml:"type"`
	Description      string                 `yaml:"description,omitempty"`
	AuthTypes        []AuthType             `yaml:"auth-types,omitempty,flow"`
	Endpoint         string                 `yaml:"endpoint,omitempty"`
	IdentityEndpoint string                 `yaml:"identity-endpoint,omitempty"`
	StorageEndpoint  string                 `yaml:"storage-endpoint,omitempty"`
	Regions          regions                `yaml:"regions,omitempty"`
	Config           map[string]interface{} `yaml:"config,omitempty"`
	RegionConfig     RegionConfig           `yaml:"region-config,omitempty"`
	CACertificates   []string               `yaml:"ca-certificates,omitempty"`
}

// regions is a collection of regions, either as a map and/or
// as a yaml.MapSlice.
//
// When marshalling, we populate the Slice field only. This is
// necessary for us to control the order of map items.
//
// When unmarshalling, we populate both Map and Slice. Map is
// populated to simplify conversion to Region objects. Slice
// is populated so we can identify the first map item, which
// becomes the default region for the cloud.
type regions struct {
	Map   map[string]*region
	Slice yaml.MapSlice
}

// region is equivalent to Region, for marshalling and unmarshalling.
type region struct {
	Endpoint         string `yaml:"endpoint,omitempty"`
	IdentityEndpoint string `yaml:"identity-endpoint,omitempty"`
	StorageEndpoint  string `yaml:"storage-endpoint,omitempty"`
}

var caasCloudTypes = map[string]bool{
	"kubernetes": true,
}

func CloudIsCAAS(cloud Cloud) bool {
	return caasCloudTypes[cloud.Type]
}

// CloudByName returns the cloud with the specified name.
// If there exists no cloud with the specified name, an
// error satisfying errors.IsNotFound will be returned.
//
// TODO(axw) write unit tests for this.
func CloudByName(name string) (*Cloud, error) {
	// Personal clouds take precedence.
	personalClouds, err := PersonalCloudMetadata()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if cloud, ok := personalClouds[name]; ok {
		return &cloud, nil
	}
	clouds, _, err := PublicCloudMetadata(JujuPublicCloudsPath())
	if err != nil {
		return nil, errors.Trace(err)
	}
	if cloud, ok := clouds[name]; ok {
		return &cloud, nil
	}
	return nil, errors.NotFoundf("cloud %s", name)
}

// RegionByName finds the region in the given slice with the
// specified name, with case folding.
func RegionByName(regions []Region, name string) (*Region, error) {
	for _, region := range regions {
		if !strings.EqualFold(region.Name, name) {
			continue
		}
		return &region, nil
	}
	return nil, errors.NewNotFound(nil, fmt.Sprintf(
		"region %q not found (expected one of %q)",
		name, RegionNames(regions),
	))
}

// RegionNames returns a sorted list of the names of the given regions.
func RegionNames(regions []Region) []string {
	names := make([]string, len(regions))
	for i, region := range regions {
		names[i] = region.Name
	}
	sort.Strings(names)
	return names
}

// JujuPublicCloudsPath is the location where public cloud information is
// expected to be found. Requires JUJU_HOME to be set.
func JujuPublicCloudsPath() string {
	return osenv.JujuXDGDataHomePath("public-clouds.yaml")
}

// PublicCloudMetadata looks in searchPath for cloud metadata files and if none
// are found, returns the fallback public cloud metadata.
func PublicCloudMetadata(searchPath ...string) (result map[string]Cloud, fallbackUsed bool, err error) {
	for _, file := range searchPath {
		data, err := ioutil.ReadFile(file)
		if err != nil && os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, false, errors.Trace(err)
		}
		clouds, err := ParseCloudMetadata(data)
		if err != nil {
			return nil, false, errors.Trace(err)
		}
		return clouds, false, err
	}
	clouds, err := ParseCloudMetadata([]byte(fallbackPublicCloudInfo))
	return clouds, true, err
}

// ParseOneCloud parses the given yaml bytes into a single Cloud metadata.
func ParseOneCloud(data []byte) (Cloud, error) {
	c := &cloud{}
	if err := yaml.Unmarshal(data, &c); err != nil {
		return Cloud{}, errors.Annotate(err, "cannot unmarshal yaml cloud metadata")
	}
	return cloudFromInternal(c), nil
}

// ParseCloudMetadata parses the given yaml bytes into Clouds metadata.
func ParseCloudMetadata(data []byte) (map[string]Cloud, error) {
	var metadata cloudSet
	if err := yaml.Unmarshal(data, &metadata); err != nil {
		return nil, errors.Annotate(err, "cannot unmarshal yaml cloud metadata")
	}

	// Translate to the exported type. For each cloud, we store
	// the first region for the cloud as its default region.
	clouds := make(map[string]Cloud)
	for name, cloud := range metadata.Clouds {
		details := cloudFromInternal(cloud)
		details.Name = name
		if details.Description == "" {
			var ok bool
			if details.Description, ok = defaultCloudDescription[name]; !ok {
				details.Description = defaultCloudDescription[cloud.Type]
			}
		}
		clouds[name] = details
	}
	return clouds, nil
}

// DefaultCloudDescription returns the description for the specified cloud
// type, or an empty string if the cloud type is unknown.
func DefaultCloudDescription(cloudType string) string {
	return defaultCloudDescription[cloudType]
}

var defaultCloudDescription = map[string]string{
	"aws":         "Amazon Web Services",
	"aws-china":   "Amazon China",
	"aws-gov":     "Amazon (USA Government)",
	"google":      "Google Cloud Platform",
	"azure":       "Microsoft Azure",
	"azure-china": "Microsoft Azure China",
	"rackspace":   "Rackspace Cloud",
	"joyent":      "Joyent Cloud",
	"cloudsigma":  "CloudSigma Cloud",
	"lxd":         "LXD Container Hypervisor",
	"maas":        "Metal As A Service",
	"openstack":   "Openstack Cloud",
	"oracle":      "Oracle Compute Cloud Service",
}

// WritePublicCloudMetadata marshals to YAML and writes the cloud metadata
// to the public cloud file.
func WritePublicCloudMetadata(cloudsMap map[string]Cloud) error {
	data, err := marshalCloudMetadata(cloudsMap)
	if err != nil {
		return errors.Trace(err)
	}
	return utils.AtomicWriteFile(JujuPublicCloudsPath(), data, 0600)
}

// IsSameCloudMetadata returns true if both meta and meta2 contain the
// same cloud metadata.
func IsSameCloudMetadata(meta1, meta2 map[string]Cloud) (bool, error) {
	// The easiest approach is to simply marshall to YAML and compare.
	yaml1, err := marshalCloudMetadata(meta1)
	if err != nil {
		return false, err
	}
	yaml2, err := marshalCloudMetadata(meta2)
	if err != nil {
		return false, err
	}
	return string(yaml1) == string(yaml2), nil
}

// marshalCloudMetadata marshals the given clouds to YAML.
func marshalCloudMetadata(cloudsMap map[string]Cloud) ([]byte, error) {
	clouds := cloudSet{make(map[string]*cloud)}
	for name, metadata := range cloudsMap {
		clouds.Clouds[name] = cloudToInternal(metadata, false)
	}
	data, err := yaml.Marshal(clouds)
	if err != nil {
		return nil, errors.Annotate(err, "cannot marshal cloud metadata")
	}
	return data, nil
}

// MarshalCloud marshals a Cloud to an opaque byte array.
func MarshalCloud(cloud Cloud) ([]byte, error) {
	return yaml.Marshal(cloudToInternal(cloud, true))
}

// UnmarshalCloud unmarshals a Cloud from a byte array produced by MarshalCloud.
func UnmarshalCloud(in []byte) (Cloud, error) {
	var internal cloud
	if err := yaml.Unmarshal(in, &internal); err != nil {
		return Cloud{}, errors.Annotate(err, "cannot unmarshal yaml cloud metadata")
	}
	return cloudFromInternal(&internal), nil
}

func cloudToInternal(in Cloud, withName bool) *cloud {
	var regions regions
	for _, r := range in.Regions {
		regions.Slice = append(regions.Slice, yaml.MapItem{
			r.Name, region{
				r.Endpoint,
				r.IdentityEndpoint,
				r.StorageEndpoint,
			},
		})
	}
	name := in.Name
	if !withName {
		name = ""
	}
	return &cloud{
		Name:             name,
		Type:             in.Type,
		AuthTypes:        in.AuthTypes,
		Endpoint:         in.Endpoint,
		IdentityEndpoint: in.IdentityEndpoint,
		StorageEndpoint:  in.StorageEndpoint,
		Regions:          regions,
		Config:           in.Config,
		RegionConfig:     in.RegionConfig,
		CACertificates:   in.CACertificates,
	}
}

func cloudFromInternal(in *cloud) Cloud {
	var regions []Region
	if len(in.Regions.Map) > 0 {
		for _, item := range in.Regions.Slice {
			name := fmt.Sprint(item.Key)
			r := in.Regions.Map[name]
			if r == nil {
				// r will be nil if none of the fields in
				// the YAML are set.
				regions = append(regions, Region{Name: name})
			} else {
				regions = append(regions, Region{
					name,
					r.Endpoint,
					r.IdentityEndpoint,
					r.StorageEndpoint,
				})
			}
		}
	}
	meta := Cloud{
		Name:             in.Name,
		Type:             in.Type,
		AuthTypes:        in.AuthTypes,
		Endpoint:         in.Endpoint,
		IdentityEndpoint: in.IdentityEndpoint,
		StorageEndpoint:  in.StorageEndpoint,
		Regions:          regions,
		Config:           in.Config,
		RegionConfig:     in.RegionConfig,
		Description:      in.Description,
		CACertificates:   in.CACertificates,
	}
	meta.denormaliseMetadata()
	return meta
}

// MarshalYAML implements the yaml.Marshaler interface.
func (r regions) MarshalYAML() (interface{}, error) {
	return r.Slice, nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (r *regions) UnmarshalYAML(f func(interface{}) error) error {
	if err := f(&r.Map); err != nil {
		return err
	}
	return f(&r.Slice)
}

// To keep the metadata concise, attributes on the metadata struct which
// have the same value for each item may be moved up to a higher level in
// the tree. denormaliseMetadata descends the tree and fills in any missing
// attributes with values from a higher level.
func (cloud Cloud) denormaliseMetadata() {
	for name, region := range cloud.Regions {
		r := region
		inherit(&r, &cloud)
		cloud.Regions[name] = r
	}
}

type structTags map[reflect.Type]map[string]int

var tagsForType structTags = make(structTags)

// RegisterStructTags ensures the yaml tags for the given structs are able to be used
// when parsing cloud metadata.
func RegisterStructTags(vals ...interface{}) {
	tags := mkTags(vals...)
	for k, v := range tags {
		tagsForType[k] = v
	}
}

func init() {
	RegisterStructTags(Cloud{}, Region{})
}

func mkTags(vals ...interface{}) map[reflect.Type]map[string]int {
	typeMap := make(map[reflect.Type]map[string]int)
	for _, v := range vals {
		t := reflect.TypeOf(v)
		typeMap[t] = yamlTags(t)
	}
	return typeMap
}

// yamlTags returns a map from yaml tag to the field index for the string fields in the given type.
func yamlTags(t reflect.Type) map[string]int {
	if t.Kind() != reflect.Struct {
		panic(errors.Errorf("cannot get yaml tags on type %s", t))
	}
	tags := make(map[string]int)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Type != reflect.TypeOf("") {
			continue
		}
		if tag := f.Tag.Get("yaml"); tag != "" {
			if i := strings.Index(tag, ","); i >= 0 {
				tag = tag[0:i]
			}
			if tag == "-" {
				continue
			}
			if tag != "" {
				f.Name = tag
			}
		}
		tags[f.Name] = i
	}
	return tags
}

// inherit sets any blank fields in dst to their equivalent values in fields in src that have matching json tags.
// The dst parameter must be a pointer to a struct.
func inherit(dst, src interface{}) {
	for tag := range tags(dst) {
		setFieldByTag(dst, tag, fieldByTag(src, tag), false)
	}
}

// tags returns the field offsets for the JSON tags defined by the given value, which must be
// a struct or a pointer to a struct.
func tags(x interface{}) map[string]int {
	t := reflect.TypeOf(x)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		panic(errors.Errorf("expected struct, not %s", t))
	}

	if tagm := tagsForType[t]; tagm != nil {
		return tagm
	}
	panic(errors.Errorf("%s not found in type table", t))
}

// fieldByTag returns the value for the field in x with the given JSON tag, or "" if there is no such field.
func fieldByTag(x interface{}, tag string) string {
	tagm := tags(x)
	v := reflect.ValueOf(x)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if i, ok := tagm[tag]; ok {
		return v.Field(i).Interface().(string)
	}
	return ""
}

// setFieldByTag sets the value for the field in x with the given JSON tag to val.
// The override parameter specifies whether the value will be set even if the original value is non-empty.
func setFieldByTag(x interface{}, tag, val string, override bool) {
	i, ok := tags(x)[tag]
	if !ok {
		return
	}
	v := reflect.ValueOf(x).Elem()
	f := v.Field(i)
	if override || f.Interface().(string) == "" {
		f.Set(reflect.ValueOf(val))
	}
}
