// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"

	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	"github.com/juju/juju/testing"
)

type containerEnvSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&containerEnvSuite{})

func (s *v2SpecsSuite) TestContainerConfigToK8sEnvConfig(c *gc.C) {

	specStr := versionHeader + `
containers:
  - name: gitlab
    image: gitlab/latest
    config:
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
        fieldRef:
          fieldPath: spec.nodeName
      thing:
         secretKeyRef:
           name: foo
           key: bar
      thing1:
         configMapKeyRef:
           name: foo
           key: bar
      secretRef:
        - name: secret1
          optional: true
        - name: secret2
      configMapRef:
        - name: configmap1
          optional: true
        - name: configmap2
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
		SecretRef: &core.SecretEnvSource{Optional: boolPtr(true)},
	}
	envFromSourceSecret1.SecretRef.Name = "secret1"

	envFromSourceSecret2 := core.EnvFromSource{
		SecretRef: &core.SecretEnvSource{},
	}
	envFromSourceSecret2.SecretRef.Name = "secret2"

	envFromSourceConfigmap1 := core.EnvFromSource{
		ConfigMapRef: &core.ConfigMapEnvSource{Optional: boolPtr(true)},
	}
	envFromSourceConfigmap1.ConfigMapRef.Name = "configmap1"

	envFromSourceConfigmap2 := core.EnvFromSource{
		ConfigMapRef: &core.ConfigMapEnvSource{},
	}
	envFromSourceConfigmap2.ConfigMapRef.Name = "configmap2"

	specs, err := k8sspecs.ParsePodSpec(specStr)
	c.Assert(err, jc.ErrorIsNil)
	envVars, envFromSource, err := k8sspecs.ContainerConfigToK8sEnvConfig(specs.Containers[0].Config)
	c.Assert(err, jc.ErrorIsNil)
	expectedEnvVar := []core.EnvVar{
		{Name: "MY_NODE_NAME", ValueFrom: &core.EnvVarSource{FieldRef: &core.ObjectFieldSelector{FieldPath: "spec.nodeName"}}},
		{Name: "attr", Value: `foo=bar; name["fred"]="blogs";`},
		{Name: "bar", Value: "true"},
		{Name: "brackets", Value: `["hello", "world"]`},
		{Name: "float", Value: "111.11111111"},
		{Name: "foo", Value: "bar"},
		{Name: "int", Value: "111"},
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
