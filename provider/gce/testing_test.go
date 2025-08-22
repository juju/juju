// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"net/url"

	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	"google.golang.org/api/compute/v1"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/provider/gce/google"
	"github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
)

// Ensure GCE provider supports the expected interfaces.
var (
	_ config.ConfigSchemaSource = (*environProvider)(nil)
)

// These values are fake GCE auth credentials for use in tests.
const (
	ClientName  = "ba9876543210-0123456789abcdefghijklmnopqrstuv"
	ClientID    = ClientName + ".apps.googleusercontent.com"
	ClientEmail = ClientName + "@developer.gserviceaccount.com"
	ProjectID   = "my-juju"
	PrivateKey  = `-----BEGIN PRIVATE KEY-----
...
...
...
...
...
...
...
...
...
...
...
...
...
...
-----END PRIVATE KEY-----
`
)

// These are fake config values for use in tests.
var (
	ConfigAttrs = testing.FakeConfig().Merge(testing.Attrs{
		"type":            "gce",
		"uuid":            "2d02eeac-9dbb-11e4-89d3-123b93f75cba",
		"controller-uuid": "bfef02f1-932a-425a-a102-62175dcabd1d",
	})
)

func MakeTestCloudSpec() environscloudspec.CloudSpec {
	cred := MakeTestCredential()
	return environscloudspec.CloudSpec{
		Type:       "gce",
		Name:       "google",
		Region:     "us-east1",
		Endpoint:   "https://www.googleapis.com",
		Credential: &cred,
	}
}

func MakeTestCredential() cloud.Credential {
	return cloud.NewCredential(
		cloud.OAuth2AuthType,
		map[string]string{
			"project-id":   ProjectID,
			"client-id":    ClientID,
			"client-email": ClientEmail,
			"private-key":  PrivateKey,
		},
	)
}

var InvalidCredentialError = &url.Error{"Get", "testbad.com", errors.New("400 Bad Request")}

type BaseSuite struct {
	jujutesting.IsolationSuite

	ControllerUUID string
	ModelUUID      string

	CallCtx                *context.CloudCallContext
	InvalidatedCredentials bool

	MockService   *MockComputeService
	StartInstArgs environs.StartInstanceParams
}

var _ environs.Environ = (*environ)(nil)
var _ simplestreams.HasRegion = (*environ)(nil)
var _ instances.Instance = (*environInstance)(nil)

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.ControllerUUID = testing.FakeControllerConfig().ControllerUUID()
	s.initInst(c)

	s.CallCtx = &context.CloudCallContext{
		InvalidateCredentialFunc: func(string) error {
			s.InvalidatedCredentials = true
			return nil
		},
	}
}

func (s *BaseSuite) Prefix(env *environ) string {
	return env.namespace.Prefix()
}

func (s *BaseSuite) SetupEnv(c *gc.C, gce *MockComputeService) *environ {
	cfg := s.NewConfig(c, nil)
	ecfg, err := newConfig(cfg, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.ModelUUID = cfg.UUID()

	ns, err := instance.NewNamespace(cfg.UUID())
	c.Assert(err, jc.ErrorIsNil)
	env := &environ{
		name:      "google",
		namespace: ns,
		cloud:     MakeTestCloudSpec(),
		gce:       gce,
		ecfg:      ecfg,
		uuid:      cfg.UUID(),
	}
	return env
}

func (s *BaseSuite) SetupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.MockService = NewMockComputeService(ctrl)
	s.AddCleanup(func(_ *gc.C) {
		s.MockService = nil
		s.InvalidatedCredentials = false
	})
	return ctrl
}

func (s *BaseSuite) initInst(c *gc.C) {
	tools := []*coretools.Tools{{
		Version: version.Binary{Arch: arch.AMD64, Release: "ubuntu"},
		URL:     "https://example.org",
	}}

	var instType = "n1-standard-1"
	cons := constraints.Value{InstanceType: &instType}

	instanceConfig, err := instancecfg.NewBootstrapInstanceConfig(testing.FakeControllerConfig(), cons, cons,
		jujuversion.DefaultSupportedLTSBase(), "", nil)
	c.Assert(err, jc.ErrorIsNil)

	err = instanceConfig.SetTools(tools)
	c.Assert(err, jc.ErrorIsNil)

	instanceConfig.Tags = map[string]string{
		tags.JujuIsController: "true",
		tags.JujuController:   s.ControllerUUID,
	}
	s.StartInstArgs = environs.StartInstanceParams{
		ControllerUUID: s.ControllerUUID,
		InstanceConfig: instanceConfig,
		Tools:          tools,
		Constraints:    cons,
	}
}

func (s *BaseSuite) NewConfig(c *gc.C, updates testing.Attrs) *config.Config {
	var err error
	cfg := testing.ModelConfig(c)
	cfg, err = cfg.Apply(ConfigAttrs)
	c.Assert(err, jc.ErrorIsNil)
	cfg, err = cfg.Apply(updates)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

func (s *BaseSuite) NewComputeInstance(c *gc.C, id string) *compute.Instance {
	inst := &compute.Instance{
		Name:   id,
		Zone:   "home-zone",
		Status: google.StatusRunning,
		ServiceAccounts: []*compute.ServiceAccount{{
			Email: "fred@foo.com",
		}},
		Disks: []*compute.AttachedDisk{{
			DiskSizeGb: 15,
		}},
	}
	return inst
}

func (s *BaseSuite) NewEnvironInstance(c *gc.C, env *environ, id string) *environInstance {
	base := s.NewComputeInstance(c, id)
	return newInstance(base, env)
}

func (s *BaseSuite) GoogleInstance(c *gc.C, inst instances.Instance) *compute.Instance {
	envInst, ok := inst.(*environInstance)
	c.Assert(ok, jc.IsTrue)
	return envInst.base
}
