// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/juju/errors"
	"github.com/kr/pretty"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/juju/juju/caas/specs"
)

type fieldSelector struct {
	APIVersion string `json:"api-version,omitempty" yaml:"api-version,omitempty"`
	Path       string `json:"path" yaml:"path"`
}

func (fs fieldSelector) to() *core.ObjectFieldSelector {
	if fs.Path == "" {
		return nil
	}
	return &core.ObjectFieldSelector{
		APIVersion: fs.APIVersion,
		FieldPath:  fs.Path,
	}
}

type fieldRef struct {
	Field *fieldSelector `json:"field" yaml:"field"`
}

func (fr fieldRef) to(name string) (out []core.EnvVar) {
	if fr.Field == nil {
		return nil
	}
	selector := fr.Field.to()
	if selector == nil {
		return
	}
	return append(out, core.EnvVar{
		Name: name,
		ValueFrom: &core.EnvVarSource{
			FieldRef: selector,
		},
	})
}

type resourceSelector struct {
	ContainerName string            `json:"container-name,omitempty" yaml:"container-name,omitempty"`
	Resource      string            `json:"resource" yaml:"resource"`
	Divisor       resource.Quantity `json:"divisor,omitempty" yaml:"divisor,omitempty"`
}

func (rs resourceSelector) to() *core.ResourceFieldSelector {
	if rs.Resource == "" {
		return nil
	}
	return &core.ResourceFieldSelector{
		ContainerName: rs.ContainerName,
		Resource:      rs.Resource,
		Divisor:       rs.Divisor,
	}
}

type resourceRef struct {
	Resource *resourceSelector `json:"resource" yaml:"resource"`
}

func (rr resourceRef) to(name string) (out []core.EnvVar) {
	if rr.Resource == nil {
		return nil
	}
	selector := rr.Resource.to()
	if selector == nil {
		return
	}
	return append(out, core.EnvVar{
		Name: name,
		ValueFrom: &core.EnvVarSource{
			ResourceFieldRef: selector,
		},
	})
}

type secretSelector struct {
	Name     string `json:"name" yaml:"name"`
	Key      string `json:"key,omitempty" yaml:"key,omitempty"`
	Optional *bool  `json:"optional,omitempty" yaml:"optional,omitempty"`
}

func (ss secretSelector) to() *core.SecretEnvSource {
	if ss.Name == "" {
		return nil
	}
	out := &core.SecretEnvSource{Optional: ss.Optional}
	out.Name = ss.Name
	return out
}

func (ss secretSelector) toKeySelector() *core.SecretKeySelector {
	if ss.Key == "" || ss.Name == "" {
		return nil
	}
	out := &core.SecretKeySelector{
		Key:      ss.Key,
		Optional: ss.Optional,
	}
	out.Name = ss.Name
	return out
}

type secretRef struct {
	Secret *secretSelector `json:"secret" yaml:"secret"`
}

func (skr secretRef) to(name string) (envVars []core.EnvVar, envFromSources []core.EnvFromSource) {
	if skr.Secret == nil {
		return
	}
	keySelector := skr.Secret.toKeySelector()
	if keySelector != nil {
		envVars = append(envVars, core.EnvVar{
			Name: name,
			ValueFrom: &core.EnvVarSource{
				SecretKeyRef: keySelector,
			},
		})
		return
	}
	selector := skr.Secret.to()
	if selector != nil {
		envFromSources = append(envFromSources, core.EnvFromSource{SecretRef: selector})
	}
	return
}

type configMapSelector struct {
	Name     string `json:"name" yaml:"name"`
	Key      string `json:"key,omitempty" yaml:"key,omitempty"`
	Optional *bool  `json:"optional,omitempty" yaml:"optional,omitempty"`
}

func (cms configMapSelector) to() *core.ConfigMapEnvSource {
	if cms.Name == "" {
		return nil
	}
	out := &core.ConfigMapEnvSource{Optional: cms.Optional}
	out.Name = cms.Name
	return out
}

func (cms configMapSelector) toKeySelector() *core.ConfigMapKeySelector {
	if cms.Key == "" || cms.Name == "" {
		return nil
	}
	out := &core.ConfigMapKeySelector{
		Key:      cms.Key,
		Optional: cms.Optional,
	}
	out.Name = cms.Name
	return out
}

type configMapRef struct {
	ConfigMap *configMapSelector `json:"config-map" yaml:"config-map"`
}

func (cmkr configMapRef) to(name string) (envVars []core.EnvVar, envFromSources []core.EnvFromSource) {
	if cmkr.ConfigMap == nil {
		return
	}
	keySelector := cmkr.ConfigMap.toKeySelector()
	if keySelector != nil {
		envVars = append(envVars, core.EnvVar{
			Name: name,
			ValueFrom: &core.EnvVarSource{
				ConfigMapKeyRef: keySelector,
			},
		})
		return
	}
	selector := cmkr.ConfigMap.to()
	if selector != nil {
		envFromSources = append(envFromSources, core.EnvFromSource{ConfigMapRef: selector})
	}
	return
}

type configValue struct {
	fieldRef    `json:",inline" yaml:",inline"`
	resourceRef `json:",inline" yaml:",inline"`

	secretRef    `json:",inline" yaml:",inline"`
	configMapRef `json:",inline" yaml:",inline"`
}

type toEnvVars interface {
	to(string) []core.EnvVar
}
type toEnvVarsAndEnvFromSources interface {
	to(string) ([]core.EnvVar, []core.EnvFromSource)
}

func (cv configValue) to(name string) (envVars []core.EnvVar, envFromSources []core.EnvFromSource, err error) {
	for _, item := range []toEnvVars{
		cv.fieldRef,
		cv.resourceRef,
	} {
		envVars = append(envVars, item.to(name)...)
	}

	for _, item := range []toEnvVarsAndEnvFromSources{
		cv.secretRef,
		cv.configMapRef,
	} {
		o1, o2 := item.to(name)
		envVars = append(envVars, o1...)
		envFromSources = append(envFromSources, o2...)
	}

	count := len(envVars) + len(envFromSources)
	if count == 1 {
		return envVars, envFromSources, nil
	}
	return nil, nil, errors.NotSupportedf("config format of %q", name)
}

// JSON Unmarshal types - https://golang.org/pkg/encoding/json/#Unmarshal
func stringify(i interface{}) (string, error) {
	type stringer interface {
		String() string
	}

	switch v := i.(type) {
	case string:
		return v, nil
	case bool:
		return strconv.FormatBool(v), nil
	case float64:
		// All the numbers are float64 - https://golang.org/pkg/encoding/json/#Number
		return fmt.Sprintf("%g", v), nil
	case stringer:
		return v.(stringer).String(), nil
	default:
		return "", errors.NotSupportedf("%v with type %T", i, i)
	}
}

func parseVal(from, into interface{}) error {
	jsonbody, err := json.Marshal(from)
	if err != nil {
		return errors.Trace(err)
	}
	decoder := newStrictYAMLOrJSONDecoder(bytes.NewReader(jsonbody), len(jsonbody))
	return decoder.Decode(&into)
}

func processMapInterfaceValue(k string, v map[string]interface{}) (envVars []core.EnvVar, envFromSources []core.EnvFromSource, err error) {
	logger.Tracef("processing container config key: %q, value(%T): %+v", k, v, v)

	var val configValue
	if err := parseVal(v, &val); err != nil {
		return nil, nil, errors.Trace(err)
	}
	envVars, envFromSources, err = val.to(k)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return envVars, envFromSources, nil
}

// ContainerConfigToK8sEnvConfig converts ContainerConfig to k8s format for container value mount.
func ContainerConfigToK8sEnvConfig(cc specs.ContainerConfig) (envVars []core.EnvVar, envFromSources []core.EnvFromSource, err error) {
	for k, v := range cc {
		// Normal key value pairs.
		if strV, err := stringify(v); err == nil {
			envVars = append(envVars, core.EnvVar{Name: k, Value: strV})
			continue
		}

		switch envVal := v.(type) {
		case map[string]interface{}:
			vars, envFroms, err := processMapInterfaceValue(k, envVal)
			if err != nil {
				logger.Tracef("processing container config %q, err -> %v", k, errors.ErrorStack(err))
				return nil, nil, errors.Trace(err)
			}
			envVars = append(envVars, vars...)
			envFromSources = append(envFromSources, envFroms...)
		default:
			return nil, nil, errors.NotSupportedf("config %q with type %T", k, v)
		}
	}

	// Sort for tests.
	sort.SliceStable(envVars, func(i, j int) bool {
		return envVars[i].Name < envVars[j].Name
	})
	sort.SliceStable(envFromSources, func(i, j int) bool {
		return getEnvFromSourceName(envFromSources[i]) < getEnvFromSourceName(envFromSources[j])
	})

	logger.Tracef("envVars -> %s", pretty.Sprint(envVars))
	logger.Tracef("envFromSources -> %s", pretty.Sprint(envFromSources))
	return envVars, envFromSources, nil
}

func getEnvFromSourceName(in core.EnvFromSource) (out string) {
	if in.ConfigMapRef != nil {
		return in.ConfigMapRef.Name
	}
	if in.SecretRef != nil {
		return in.SecretRef.Name
	}
	return out
}
