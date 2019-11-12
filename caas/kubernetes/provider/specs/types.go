// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	core "k8s.io/api/core/v1"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"

	"github.com/juju/juju/caas/specs"
)

var logger = loggo.GetLogger("juju.kubernetes.provider.specs")

type (
	// K8sPodSpec is the current k8s pod spec.
	K8sPodSpec = K8sPodSpecV2
)

type k8sContainer struct {
	specs.ContainerSpec `json:",inline" yaml:",inline"`
	Kubernetes          *K8sContainerSpec `json:"kubernetes,omitempty" yaml:"kubernetes,omitempty"`
}

// Validate validates k8sContainer.
func (c *k8sContainer) Validate() error {
	if err := c.ContainerSpec.Validate(); err != nil {
		return errors.Trace(err)
	}
	if c.Kubernetes != nil {
		if err := c.Kubernetes.Validate(); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

type k8sContainerInterface interface {
	Validate() error
	ToContainerSpec() specs.ContainerSpec
}

func (c *k8sContainer) ToContainerSpec() specs.ContainerSpec {
	quoteStrings(c.Config)
	result := specs.ContainerSpec{
		ImageDetails:    c.ImageDetails,
		Name:            c.Name,
		Init:            c.Init,
		Image:           c.Image,
		Ports:           c.Ports,
		Command:         c.Command,
		Args:            c.Args,
		WorkingDir:      c.WorkingDir,
		Config:          c.Config,
		Files:           c.Files,
		ImagePullPolicy: c.ImagePullPolicy,
	}
	if c.Kubernetes != nil {
		result.ProviderContainer = c.Kubernetes
	}
	return result
}

// K8sContainerSpec is a subset of v1.Container which defines
// attributes we expose for charms to set.
type K8sContainerSpec struct {
	LivenessProbe   *core.Probe           `json:"livenessProbe,omitempty" yaml:"livenessProbe,omitempty"`
	ReadinessProbe  *core.Probe           `json:"readinessProbe,omitempty" yaml:"readinessProbe,omitempty"`
	SecurityContext *core.SecurityContext `json:"securityContext,omitempty" yaml:"securityContext,omitempty"`
}

// Validate validates K8sContainerSpec.
func (*K8sContainerSpec) Validate() error {
	return nil
}

// PodSpec is a subset of v1.PodSpec which defines
// attributes we expose for charms to set.
type PodSpec struct {
	RestartPolicy                 core.RestartPolicy       `json:"restartPolicy,omitempty" yaml:"restartPolicy,omitempty"`
	ActiveDeadlineSeconds         *int64                   `json:"activeDeadlineSeconds,omitempty" yaml:"activeDeadlineSeconds,omitempty"`
	TerminationGracePeriodSeconds *int64                   `json:"terminationGracePeriodSeconds,omitempty" yaml:"terminationGracePeriodSeconds,omitempty"`
	SecurityContext               *core.PodSecurityContext `json:"securityContext,omitempty" yaml:"securityContext,omitempty"`
	ReadinessGates                []core.PodReadinessGate  `json:"readinessGates,omitempty" yaml:"readinessGates,omitempty"`
	DNSPolicy                     core.DNSPolicy           `json:"dnsPolicy,omitempty" yaml:"dnsPolicy,omitempty"`
}

// IsEmpty checks if PodSpec is empty or not.
func (ps PodSpec) IsEmpty() bool {
	return ps.RestartPolicy == "" &&
		ps.ActiveDeadlineSeconds == nil &&
		ps.TerminationGracePeriodSeconds == nil &&
		ps.SecurityContext == nil &&
		len(ps.ReadinessGates) == 0 &&
		ps.DNSPolicy == ""
}

var boolValues = set.NewStrings(
	strings.Split("y|Y|yes|Yes|YES|n|N|no|No|NO|true|True|TRUE|false|False|FALSE|on|On|ON|off|Off|OFF", "|")...)

var specialValues = ":{}[],&*#?|-<>=!%@`"

func quoteStrings(config map[string]interface{}) {
	// Any string config values that could be interpreted as bools
	// or which contain special YAML chars need to be quoted.
	for k, v := range config {
		strValue, ok := v.(string)
		if !ok {
			continue
		}
		if boolValues.Contains(strValue) || strings.IndexAny(strValue, specialValues) >= 0 {
			strValue = strings.Replace(strValue, "'", "''", -1)
			config[k] = fmt.Sprintf("'%s'", strValue)
		}
	}
}

type k8sContainers struct {
	Containers []k8sContainer `json:"containers" yaml:"containers"`
}

// Validate is defined on ProviderContainer.
func (cs *k8sContainers) Validate() error {
	if len(cs.Containers) == 0 {
		return errors.New("require at least one container spec")
	}
	for _, c := range cs.Containers {
		if err := c.Validate(); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

type k8sContainersInterface interface {
	Validate() error
}

func parseContainers(in string, containerSpec k8sContainersInterface) error {
	decoder := newStrictYAMLOrJSONDecoder(strings.NewReader(in), len(in))
	if err := decoder.Decode(containerSpec); err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(containerSpec.Validate())
}

// ParsePodSpec parses a YAML file which defines how to
// configure a CAAS pod. We allow for generic container
// set up plus k8s select specific features.
func ParsePodSpec(in string) (*specs.PodSpec, error) {
	return parsePodSpec(in, getParser)
}

//go:generate mockgen -package mocks -destination ./mocks/parsers_mock.go github.com/juju/juju/caas/kubernetes/provider/specs PodSpecConverter
func parsePodSpec(
	in string,
	getParser func(specVersion specs.Version) (parserType, error),
) (*specs.PodSpec, error) {
	version, err := specs.GetVersion(in)
	if err != nil {
		return nil, errors.Trace(err)
	}
	parser, err := getParser(version)
	if err != nil {
		return nil, errors.Trace(err)
	}
	spec, err := parser(in)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err = spec.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	return spec.ToLatest(), nil
}

type parserType func(string) (PodSpecConverter, error)

// PodSpecConverter defines methods to validate and convert a specific version of podspec to latest version.
type PodSpecConverter interface {
	Validate() error
	ToLatest() *specs.PodSpec
}

func getParser(specVersion specs.Version) (parserType, error) {
	switch specVersion {
	case specs.Version2:
		return parsePodSpecV2, nil
	case specs.VersionLegacy:
		return parsePodSpecLegacy, nil
	default:
		return nil, errors.NewNotSupported(nil, fmt.Sprintf("latest supported version %d, but got podspec version %d", specs.CurrentVersion, specVersion))
	}
}

// YAMLOrJSONDecoder attempts to decode a stream of JSON documents or
// YAML documents by sniffing for a leading { character.
type YAMLOrJSONDecoder struct {
	bufferSize int
	r          io.Reader
	rawData    []byte

	strict bool
}

func newStrictYAMLOrJSONDecoder(r io.Reader, bufferSize int) *YAMLOrJSONDecoder {
	return newYAMLOrJSONDecoder(r, bufferSize, true)
}

func newYAMLOrJSONDecoder(r io.Reader, bufferSize int, strict bool) *YAMLOrJSONDecoder {
	return &YAMLOrJSONDecoder{
		r:          r,
		bufferSize: bufferSize,
		strict:     strict,
	}
}

func (d *YAMLOrJSONDecoder) jsonify() (err error) {
	buffer := bufio.NewReaderSize(d.r, d.bufferSize)
	rawData, _ := buffer.Peek(d.bufferSize)
	if rawData, err = k8syaml.ToJSON(rawData); err != nil {
		return errors.Trace(err)
	}
	d.r = bytes.NewReader(rawData)
	d.rawData = rawData
	return nil
}

func (d *YAMLOrJSONDecoder) processError(err error, decoder *json.Decoder) error {
	syntax, ok := err.(*json.SyntaxError)
	if !ok {
		return err
	}
	data, readErr := ioutil.ReadAll(decoder.Buffered())
	if readErr != nil {
		logger.Debugf("reading stream failed: %v", readErr)
	}
	jsonData := string(data)

	// if contents from io.Reader are not complete,
	// use the original raw data to prevent panic
	if int64(len(jsonData)) <= syntax.Offset {
		jsonData = string(d.rawData)
	}

	start := strings.LastIndex(jsonData[:syntax.Offset], "\n") + 1
	line := strings.Count(jsonData[:start], "\n")
	return k8syaml.JSONSyntaxError{
		Line: line,
		Err:  fmt.Errorf(syntax.Error()),
	}
}

// Decode unmarshals the next object from the underlying stream into the
// provide object, or returns an error.
func (d *YAMLOrJSONDecoder) Decode(into interface{}) error {
	if err := d.jsonify(); err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("decoding stream as JSON")
	decoder := json.NewDecoder(d.r)
	if d.strict {
		decoder.DisallowUnknownFields()
	}
	return d.processError(decoder.Decode(into), decoder)
}
