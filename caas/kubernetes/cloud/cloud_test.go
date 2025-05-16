// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"strings"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	k8scloud "github.com/juju/juju/caas/kubernetes/cloud"
	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/cloud"
)

type cloudSuite struct {
}

func TestCloudSuite(t *stdtesting.T) { tc.Run(t, &cloudSuite{}) }
func (s *cloudSuite) TestConfigFromReader(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
	_, exists := conf.Contexts["jujukube"]
	c.Assert(exists, tc.IsTrue)
	_, exists = conf.Clusters["jujukube"]
	c.Assert(exists, tc.IsTrue)
	_, exists = conf.AuthInfos["wallyworld"]
	c.Assert(exists, tc.IsTrue)
}

func (s *cloudSuite) TestCloudsFromKubeConfigContexts(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
	clouds, err := k8scloud.CloudsFromKubeConfigContexts(conf)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(clouds), tc.Equals, 2)

	foundCloud1 := false
	for _, cloud := range clouds {
		if cloud.Name == "jujukube" {
			foundCloud1 = true
		}
	}
	c.Assert(foundCloud1, tc.IsTrue)
	foundCloud2 := false
	for _, cloud := range clouds {
		if cloud.Name == "jujukube1" {
			foundCloud2 = true
		}
	}
	c.Assert(foundCloud2, tc.IsTrue)
}

func (s *cloudSuite) TestCloudFromKubeConfigContext(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
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
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(cl.Name, tc.Equals, "test1")
	c.Assert(cl.Type, tc.Equals, constants.CAASProviderType)
	c.Assert(cl.HostCloudRegion, tc.Equals, "jujutest")
	c.Assert(cl.Description, tc.Equals, "description")
	c.Assert(cl.Regions, tc.DeepEquals, []cloud.Region{
		{
			Name: "juju test",
		},
	})
}

func (s *cloudSuite) TestCloudFromKubeConfigContextReader(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cl.Name, tc.Equals, "test1")
	c.Assert(cl.Type, tc.Equals, constants.CAASProviderType)
	c.Assert(cl.HostCloudRegion, tc.Equals, "jujutest")
	c.Assert(cl.Description, tc.Equals, "description")
	c.Assert(cl.Regions, tc.DeepEquals, []cloud.Region{
		{
			Name: "juju test",
		},
	})
}

func (s *cloudSuite) CloudFromKubeConfigCluster(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
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
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(cl.Name, tc.Equals, "test1")
	c.Assert(cl.Type, tc.Equals, constants.CAASProviderType)
	c.Assert(cl.HostCloudRegion, tc.Equals, "jujutest")
	c.Assert(cl.Description, tc.Equals, "description")
	c.Assert(cl.Regions, tc.DeepEquals, []cloud.Region{
		{
			Name: "juju test",
		},
	})
}

func (s *cloudSuite) TestCloudFromKubeConfigContextDoesNotExist(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
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
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *cloudSuite) TestCloudFromKubeConfigContextClusterDoesNotExist(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
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
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *cloudSuite) CloudFromKubeConfigClusterReader(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(cl.Name, tc.Equals, "test1")
	c.Assert(cl.Type, tc.Equals, constants.CAASProviderType)
	c.Assert(cl.HostCloudRegion, tc.Equals, "jujutest")
	c.Assert(cl.Description, tc.Equals, "description")
	c.Assert(cl.Regions, tc.DeepEquals, []cloud.Region{
		{
			Name: "juju test",
		},
	})
}

func (s *cloudSuite) CloudFromKubeConfigClusterNotExist(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
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
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}
