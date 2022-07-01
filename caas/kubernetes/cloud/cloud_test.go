// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"strings"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	k8scloud "github.com/juju/juju/v3/caas/kubernetes/cloud"
	"github.com/juju/juju/v3/caas/kubernetes/provider/constants"
	"github.com/juju/juju/v3/cloud"
)

type cloudSuite struct {
}

var _ = gc.Suite(&cloudSuite{})

func (s *cloudSuite) TestConfigFromReader(c *gc.C) {
	rawConf := `
apiVersion: v1
clusters:
- cluster:
    server: https://localhost:8443
  name: jujukube
contexts:
- context:
    cluster: jujukube
    namespace: juju-controller
    user: wallyworld
  name: jujukube
current-context: jujukube
kind: Config
preferences: {}
users:
- name: wallyworld
  user:
    username: wallyworld
    password: jujurocks
`

	conf, err := k8scloud.ConfigFromReader(strings.NewReader(rawConf))
	c.Assert(err, jc.ErrorIsNil)
	_, exists := conf.Contexts["jujukube"]
	c.Assert(exists, jc.IsTrue)
	_, exists = conf.Clusters["jujukube"]
	c.Assert(exists, jc.IsTrue)
	_, exists = conf.AuthInfos["wallyworld"]
	c.Assert(exists, jc.IsTrue)
}

func (s *cloudSuite) TestCloudsFromKubeConfigContexts(c *gc.C) {
	rawConf := `
apiVersion: v1
clusters:
- cluster:
    server: https://localhost:8443
  name: jujukube
- cluster:
    server: https://localhost:8443
  name: jujukube1
contexts:
- context:
    cluster: jujukube
    namespace: juju-controller
    user: wallyworld
  name: jujukube
- context:
    cluster: jujukube1
    namespace: juju-controller
    user: tlm
  name: jujukube1
current-context: jujukube
kind: Config
preferences: {}
users:
- name: wallyworld
  user:
    username: wallyworld
    password: jujurocks
- name: tlm
  user:
    username: tlm
    password: jujurocks
`

	conf, err := k8scloud.ConfigFromReader(strings.NewReader(rawConf))
	c.Assert(err, jc.ErrorIsNil)
	clouds, err := k8scloud.CloudsFromKubeConfigContexts(conf)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(clouds), gc.Equals, 2)

	foundCloud1 := false
	for _, cloud := range clouds {
		if cloud.Name == "jujukube" {
			foundCloud1 = true
		}
	}
	c.Assert(foundCloud1, jc.IsTrue)
	foundCloud2 := false
	for _, cloud := range clouds {
		if cloud.Name == "jujukube1" {
			foundCloud2 = true
		}
	}
	c.Assert(foundCloud2, jc.IsTrue)
}

func (s *cloudSuite) TestCloudFromKubeConfigContext(c *gc.C) {
	rawConf := `
apiVersion: v1
clusters:
- cluster:
    server: https://localhost:8443
  name: jujukube
contexts:
- context:
    cluster: jujukube
    namespace: juju-controller
    user: wallyworld
  name: jujukube
current-context: jujukube
kind: Config
preferences: {}
users:
- name: tlm
  user:
    username: wallyworld
    password: jujurocks
`

	conf, err := k8scloud.ConfigFromReader(strings.NewReader(rawConf))
	c.Assert(err, jc.ErrorIsNil)
	cl, err := k8scloud.CloudFromKubeConfigContext(
		"jujukube",
		conf,
		k8scloud.CloudParamaters{
			Name:            "test1",
			Description:     "description",
			HostCloudRegion: "jujutest",
			Regions: []cloud.Region{
				{
					Name: "juju test",
				},
			},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(cl.Name, gc.Equals, "test1")
	c.Assert(cl.Type, gc.Equals, constants.CAASProviderType)
	c.Assert(cl.HostCloudRegion, gc.Equals, "jujutest")
	c.Assert(cl.Description, gc.Equals, "description")
	c.Assert(cl.Regions, jc.DeepEquals, []cloud.Region{
		{
			Name: "juju test",
		},
	})
}

func (s *cloudSuite) TestCloudFromKubeConfigContextReader(c *gc.C) {
	rawConf := `
apiVersion: v1
clusters:
- cluster:
    server: https://localhost:8443
  name: jujukube
contexts:
- context:
    cluster: jujukube
    namespace: juju-controller
    user: wallyworld
  name: jujukube
current-context: jujukube
kind: Config
preferences: {}
users:
- name: tlm
  user:
    username: wallyworld
    password: jujurocks
`

	cl, err := k8scloud.CloudFromKubeConfigContextReader(
		"jujukube",
		strings.NewReader(rawConf),
		k8scloud.CloudParamaters{
			Name:            "test1",
			Description:     "description",
			HostCloudRegion: "jujutest",
			Regions: []cloud.Region{
				{
					Name: "juju test",
				},
			},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cl.Name, gc.Equals, "test1")
	c.Assert(cl.Type, gc.Equals, constants.CAASProviderType)
	c.Assert(cl.HostCloudRegion, gc.Equals, "jujutest")
	c.Assert(cl.Description, gc.Equals, "description")
	c.Assert(cl.Regions, jc.DeepEquals, []cloud.Region{
		{
			Name: "juju test",
		},
	})
}

func (s *cloudSuite) CloudFromKubeConfigCluster(c *gc.C) {
	rawConf := `
apiVersion: v1
clusters:
- cluster:
    server: https://localhost:8443
  name: jujukube
contexts:
- context:
    cluster: jujukube
    namespace: juju-controller
    user: wallyworld
  name: jujukube
current-context: jujukube
kind: Config
preferences: {}
users:
- name: tlm
  user:
    username: wallyworld
    password: jujurocks
`

	conf, err := k8scloud.ConfigFromReader(strings.NewReader(rawConf))
	c.Assert(err, jc.ErrorIsNil)
	cl, err := k8scloud.CloudFromKubeConfigCluster(
		"jujukube",
		conf,
		k8scloud.CloudParamaters{
			Name:            "test1",
			Description:     "description",
			HostCloudRegion: "jujutest",
			Regions: []cloud.Region{
				{
					Name: "juju test",
				},
			},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(cl.Name, gc.Equals, "test1")
	c.Assert(cl.Type, gc.Equals, constants.CAASProviderType)
	c.Assert(cl.HostCloudRegion, gc.Equals, "jujutest")
	c.Assert(cl.Description, gc.Equals, "description")
	c.Assert(cl.Regions, jc.DeepEquals, []cloud.Region{
		{
			Name: "juju test",
		},
	})
}

func (s *cloudSuite) TestCloudFromKubeConfigContextDoesNotExist(c *gc.C) {
	rawConf := `
apiVersion: v1
clusters:
- cluster:
    server: https://localhost:8443
  name: jujukube
contexts:
- context:
    cluster: jujukube
    namespace: juju-controller
    user: wallyworld
  name: jujukube
current-context: jujukube
kind: Config
preferences: {}
users:
- name: tlm
  user:
    username: wallyworld
    password: jujurocks
`

	conf, err := k8scloud.ConfigFromReader(strings.NewReader(rawConf))
	c.Assert(err, jc.ErrorIsNil)
	_, err = k8scloud.CloudFromKubeConfigContext(
		"jujukube-doest-not-exist",
		conf,
		k8scloud.CloudParamaters{
			Name:            "test1",
			Description:     "description",
			HostCloudRegion: "jujutest",
			Regions: []cloud.Region{
				{
					Name: "juju test",
				},
			},
		},
	)
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *cloudSuite) TestCloudFromKubeConfigContextClusterDoesNotExist(c *gc.C) {
	rawConf := `
apiVersion: v1
clusters:
- cluster:
    server: https://localhost:8443
  name: jujukube
contexts:
- context:
    cluster: jujukube-does-not-exist
    namespace: juju-controller
    user: wallyworld
  name: jujukube
current-context: jujukube
kind: Config
preferences: {}
users:
- name: tlm
  user:
    username: wallyworld
    password: jujurocks
`

	conf, err := k8scloud.ConfigFromReader(strings.NewReader(rawConf))
	c.Assert(err, jc.ErrorIsNil)
	_, err = k8scloud.CloudFromKubeConfigContext(
		"jujukube",
		conf,
		k8scloud.CloudParamaters{
			Name:            "test1",
			Description:     "description",
			HostCloudRegion: "jujutest",
			Regions: []cloud.Region{
				{
					Name: "juju test",
				},
			},
		},
	)
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *cloudSuite) CloudFromKubeConfigClusterReader(c *gc.C) {
	rawConf := `
apiVersion: v1
clusters:
- cluster:
    server: https://localhost:8443
  name: jujukube
contexts:
- context:
    cluster: jujukube
    namespace: juju-controller
    user: wallyworld
  name: jujukube
current-context: jujukube
kind: Config
preferences: {}
users:
- name: tlm
  user:
    username: wallyworld
    password: jujurocks
`

	cl, err := k8scloud.CloudFromKubeConfigClusterReader(
		"jujukube",
		strings.NewReader(rawConf),
		k8scloud.CloudParamaters{
			Name:            "test1",
			Description:     "description",
			HostCloudRegion: "jujutest",
			Regions: []cloud.Region{
				{
					Name: "juju test",
				},
			},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(cl.Name, gc.Equals, "test1")
	c.Assert(cl.Type, gc.Equals, constants.CAASProviderType)
	c.Assert(cl.HostCloudRegion, gc.Equals, "jujutest")
	c.Assert(cl.Description, gc.Equals, "description")
	c.Assert(cl.Regions, jc.DeepEquals, []cloud.Region{
		{
			Name: "juju test",
		},
	})
}

func (s *cloudSuite) CloudFromKubeConfigClusterNotExist(c *gc.C) {
	rawConf := `
apiVersion: v1
clusters:
- cluster:
    server: https://localhost:8443
  name: jujukube
contexts:
- context:
    cluster: jujukube
    namespace: juju-controller
    user: wallyworld
  name: jujukube
current-context: jujukube
kind: Config
preferences: {}
users:
- name: tlm
  user:
    username: wallyworld
    password: jujurocks
`

	conf, err := k8scloud.ConfigFromReader(strings.NewReader(rawConf))
	c.Assert(err, jc.ErrorIsNil)
	_, err = k8scloud.CloudFromKubeConfigCluster(
		"does-not-exist",
		conf,
		k8scloud.CloudParamaters{
			Name:            "test1",
			Description:     "description",
			HostCloudRegion: "jujutest",
			Regions: []cloud.Region{
				{
					Name: "juju test",
				},
			},
		},
	)
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}
