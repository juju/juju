// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/pointer"

	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	"github.com/juju/juju/testing"
)

type containerEnvSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&containerEnvSuite{})

func (s *containerEnvSuite) TestContainerConfigToK8sEnvConfig(c *gc.C) {

	specStr := version3Header + `
containers:
  - name: gitlab
    image: gitlab/latest
    envConfig:
      attr: foo=bar; name["fred"]="blogs";
      foo: bar
      brackets: '["hello", "world"]'
      restricted: 'yes'
      switch: on
      bar: true
      special: p@ssword's
      foo: bar
      float: 111.11111111
      int: 111
      MY_NODE_NAME:
        field:
          path: spec.nodeName
          api-version: v1
      my-resource-limit:
        resource:
          container-name: container1
          resource: requests.cpu
          divisor: 1m
      thing:
        secret:
          name: foo
          key: bar
      a-secret:
        secret:
          name: secret1
          optional: true
      another-secret:
        secret:
          name: secret2
      thing1:
        config-map:
          name: foo
          key: bar
      a-configmap:
        config-map:
          name: configmap1
          optional: true
      another-configmap:
        config-map:
          name: configmap2
`[1:]

	envVarThing := core.EnvVar{
		Name: "thing",
		ValueFrom: &core.EnvVarSource{
			SecretKeyRef: &core.SecretKeySelector{Key: "bar"},
		},
	}
	envVarThing.ValueFrom.SecretKeyRef.Name = "foo"

	envVarThing1 := core.EnvVar{
		Name: "thing1",
		ValueFrom: &core.EnvVarSource{
			ConfigMapKeyRef: &core.ConfigMapKeySelector{Key: "bar"},
		},
	}
	envVarThing1.ValueFrom.ConfigMapKeyRef.Name = "foo"

	envFromSourceSecret1 := core.EnvFromSource{
		SecretRef: &core.SecretEnvSource{Optional: pointer.BoolPtr(true)},
	}
	envFromSourceSecret1.SecretRef.Name = "secret1"

	envFromSourceSecret2 := core.EnvFromSource{
		SecretRef: &core.SecretEnvSource{},
	}
	envFromSourceSecret2.SecretRef.Name = "secret2"

	envFromSourceConfigmap1 := core.EnvFromSource{
		ConfigMapRef: &core.ConfigMapEnvSource{Optional: pointer.BoolPtr(true)},
	}
	envFromSourceConfigmap1.ConfigMapRef.Name = "configmap1"

	envFromSourceConfigmap2 := core.EnvFromSource{
		ConfigMapRef: &core.ConfigMapEnvSource{},
	}
	envFromSourceConfigmap2.ConfigMapRef.Name = "configmap2"

	specs, err := k8sspecs.ParsePodSpec(specStr)
	c.Assert(err, jc.ErrorIsNil)
	envVars, envFromSource, err := k8sspecs.ContainerConfigToK8sEnvConfig(specs.Containers[0].EnvConfig)
	c.Assert(err, jc.ErrorIsNil)
	expectedEnvVar := []core.EnvVar{
		{
			Name: "MY_NODE_NAME",
			ValueFrom: &core.EnvVarSource{
				FieldRef: &core.ObjectFieldSelector{
					FieldPath:  "spec.nodeName",
					APIVersion: "v1",
				},
			},
		},
		{Name: "attr", Value: `foo=bar; name["fred"]="blogs";`},
		{Name: "bar", Value: "true"},
		{Name: "brackets", Value: `["hello", "world"]`},
		{Name: "float", Value: "111.11111111"},
		{Name: "foo", Value: "bar"},
		{Name: "int", Value: "111"},
		{
			Name: "my-resource-limit",
			ValueFrom: &core.EnvVarSource{
				ResourceFieldRef: &core.ResourceFieldSelector{
					ContainerName: "container1",
					Resource:      "requests.cpu",
					Divisor:       resource.MustParse("1m"),
				},
			},
		},
		{Name: "restricted", Value: "yes"},
		{Name: "special", Value: "p@ssword's"},
		{Name: "switch", Value: "true"},
		envVarThing,
		envVarThing1,
	}
	expectedEnvFromSource := []core.EnvFromSource{
		envFromSourceConfigmap1,
		envFromSourceConfigmap2,
		envFromSourceSecret1,
		envFromSourceSecret2,
	}
	for i := range envVars {
		c.Check(envVars[i], jc.DeepEquals, expectedEnvVar[i])
	}
	for i := range envFromSource {
		c.Check(envFromSource[i], jc.DeepEquals, expectedEnvFromSource[i])
	}
}

func (s *containerEnvSuite) TestContainerConfigToK8sEnvConfigSliceNotSupported(c *gc.C) {
	_, _, err := k8sspecs.ContainerConfigToK8sEnvConfig(map[string]interface{}{
		"a-slice": []interface{}{},
	})
	c.Assert(err, gc.ErrorMatches, `config "a-slice" with type .* not supported`)
}

func (s *containerEnvSuite) TestContainerConfigToK8sEnvConfigFailedBadField(c *gc.C) {
	_, _, err := k8sspecs.ContainerConfigToK8sEnvConfig(map[string]interface{}{
		"a-bad-config-map": map[string]interface{}{
			"config-map": map[string]interface{}{
				"a-bad-field": "",
			},
		},
	})
	c.Assert(err, gc.ErrorMatches, `json: unknown field "a-bad-field"`)
}

func (s *containerEnvSuite) TestContainerConfigToK8sEnvConfigFailedRequiredFieldMissing(c *gc.C) {
	type tc struct {
		resourceType  string
		optionalField string
	}
	for i, t := range []tc{
		{resourceType: "secret", optionalField: "key"},
		{resourceType: "config-map", optionalField: "key"},
		{resourceType: "resource", optionalField: "container-name"},
		{resourceType: "field", optionalField: "api-version"},
	} {
		c.Logf("checking %d: %q", i, t.resourceType)
		_, _, err := k8sspecs.ContainerConfigToK8sEnvConfig(map[string]interface{}{
			"empty": map[string]interface{}{
				t.resourceType: map[string]interface{}{},
			},
		})
		c.Check(err, gc.ErrorMatches, `config format of "empty" not supported`)

		_, _, err = k8sspecs.ContainerConfigToK8sEnvConfig(map[string]interface{}{
			"empty": map[string]interface{}{
				t.resourceType: map[string]interface{}{
					t.optionalField: "foo",
				},
			},
		})
		c.Check(err, gc.ErrorMatches, `config format of "empty" not supported`)
	}
}
