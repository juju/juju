// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package equinix

import (
	"context"

	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"github.com/packethost/packngo"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cloudconfig/instancecfg"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/provider/equinix/mocks"
	"github.com/juju/juju/testing"
)

type environProviderSuite struct {
	jtesting.IsolationSuite
	provider environs.EnvironProvider
	spec     environscloudspec.CloudSpec
}

var _ = gc.Suite(&environProviderSuite{})

func (s *environProviderSuite) SetUpSuite(c *gc.C) {
	s.IsolationSuite.SetUpSuite(c)
}

func (s *environProviderSuite) TearDownSuite(c *gc.C) {
	s.IsolationSuite.TearDownSuite(c)
}

func (s *environProviderSuite) TearDownTest(c *gc.C) {
	s.IsolationSuite.TearDownTest(c)
}

func (s *environProviderSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.provider = NewProvider()
	s.spec = environscloudspec.CloudSpec{
		Type:       "equnix",
		Name:       "equnix metal",
		Region:     "am",
		Endpoint:   "https://api.packet.net/",
		Credential: fakeServicePrincipalCredential(),
	}
}

func fakeServicePrincipalCredential() *cloud.Credential {
	cred := cloud.NewCredential(
		"service-principal-secret",
		map[string]string{
			"project-id": "12345c2a-6789-4d4f-a3c4-7367d6b7cca8",
			"api-token":  "some-token",
		},
	)
	return &cred
}

func (s *environProviderSuite) TestPrepareConfig(c *gc.C) {
	cfg := makeTestModelConfig(c)
	cfg, err := s.provider.PrepareConfig(environs.PrepareConfigParams{
		Cloud:  s.spec,
		Config: cfg,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cfg, gc.NotNil)
}

func (s *environProviderSuite) TestOpen(c *gc.C) {
	env, err := environs.Open(context.Background(), s.provider, environs.OpenParams{
		Cloud:  s.spec,
		Config: makeTestModelConfig(c),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.NotNil)
}

func (s *environProviderSuite) TestDestroy(c *gc.C) {
	cntrl := gomock.NewController(c)
	defer cntrl.Finish()
	device := mocks.NewMockDeviceService(cntrl)
	device.EXPECT().Delete(gomock.Eq("2"), gomock.Eq(true)).Times(2)
	device.EXPECT().Delete(gomock.Eq("1"), gomock.Eq(true)).Times(2)
	device.EXPECT().List(gomock.Eq("12345c2a-6789-4d4f-a3c4-7367d6b7cca8"), nil).
		Return([]packngo.Device{
			{
				ID:   "1",
				Tags: []string{"juju-model-uuid=deadbeef-0bad-400d-8000-4b1d0d06f00d"},
			},
			{
				ID:   "2",
				Tags: []string{"juju-model-uuid=deadbeef-0bad-400d-8000-4b1d0d06f00d"},
			},
		}, nil, nil).AnyTimes()
	s.PatchValue(&equinixClient, func(spec environscloudspec.CloudSpec) *packngo.Client {
		cl := &packngo.Client{}
		cl.Devices = device
		return cl
	})
	env, err := environs.Open(context.Background(), s.provider, environs.OpenParams{
		Cloud:  s.spec,
		Config: makeTestModelConfig(c),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.NotNil)
	err = env.Destroy(envcontext.WithoutCredentialInvalidator(context.Background()))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environProviderSuite) TestGetPacketInstancesByTag(c *gc.C) {
	cntrl := gomock.NewController(c)
	defer cntrl.Finish()
	device := mocks.NewMockDeviceService(cntrl)
	device.EXPECT().Delete(gomock.Eq("1"), gomock.Eq(true))
	device.EXPECT().List(gomock.Eq("12345c2a-6789-4d4f-a3c4-7367d6b7cca8"), nil).
		Return([]packngo.Device{
			{
				ID: "1",
				Tags: []string{
					"juju-is-controller=true",
					"juju-controller-uuid=deadbeef-0bad-400d-8000-4b1d0d06f00d",
				},
			},
			// This controller has a different model-uuid and should be ignored.
			{
				ID: "42",
				Tags: []string{
					"juju-is-controller=true",
					"juju-controller-uuid=this-is-not-the-controller-you-are-looking-for",
				},
			},
		}, nil, nil).AnyTimes()
	s.PatchValue(&equinixClient, func(spec environscloudspec.CloudSpec) *packngo.Client {
		cl := &packngo.Client{}
		cl.Devices = device
		return cl
	})
	env, err := environs.Open(context.Background(), s.provider, environs.OpenParams{
		Cloud:  s.spec,
		Config: makeTestModelConfig(c),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.NotNil)

	err = env.DestroyController(envcontext.WithoutCredentialInvalidator(context.Background()), "deadbeef-0bad-400d-8000-4b1d0d06f00d")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environProviderSuite) TestAllInstances(c *gc.C) {
	cntrl := gomock.NewController(c)
	defer cntrl.Finish()
	device := mocks.NewMockDeviceService(cntrl)
	device.EXPECT().List(gomock.Eq("12345c2a-6789-4d4f-a3c4-7367d6b7cca8"), nil).Return([]packngo.Device{
		{
			ID:   "1",
			Tags: []string{"juju-model-uuid=deadbeef-0bad-400d-8000-4b1d0d06f00d"},
		},
		{
			ID:   "2",
			Tags: []string{"juju-model-uuid=deadbeef-0bad-400d-8000-4b1d0d06f00d"},
		},
		{
			ID:   "3",
			Tags: []string{"juju-model-uuid=none"},
		},
	}, nil, nil)
	s.PatchValue(&equinixClient, func(spec environscloudspec.CloudSpec) *packngo.Client {
		cl := &packngo.Client{}
		cl.Devices = device
		return cl
	})
	env, err := environs.Open(context.Background(), s.provider, environs.OpenParams{
		Cloud:  s.spec,
		Config: makeTestModelConfig(c),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.NotNil)
	ii, err := env.AllInstances(envcontext.WithoutCredentialInvalidator(context.Background()))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(ii) == 2, jc.IsTrue)
}

func (s *environProviderSuite) TestInstances(c *gc.C) {
	cntrl := gomock.NewController(c)
	defer cntrl.Finish()
	device := mocks.NewMockDeviceService(cntrl)
	device.EXPECT().Get(gomock.Eq("10"), nil).Return(&packngo.Device{
		ID:    "10",
		Tags:  []string{"juju-model-uuid=deadbeef-0bad-400d-8000-4b1d0d06f00d"},
		State: "active",
	}, nil, nil)
	s.PatchValue(&equinixClient, func(spec environscloudspec.CloudSpec) *packngo.Client {
		cl := &packngo.Client{}
		cl.Devices = device
		return cl
	})
	env, err := environs.Open(context.Background(), s.provider, environs.OpenParams{
		Cloud:  s.spec,
		Config: makeTestModelConfig(c),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.NotNil)
	ii, err := env.Instances(envcontext.WithoutCredentialInvalidator(context.Background()), []instance.Id{"10"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(ii[0].Id()), jc.Contains, "10")
}

func (s *environProviderSuite) TestStopInstance(c *gc.C) {
	cntrl := gomock.NewController(c)
	defer cntrl.Finish()
	device := mocks.NewMockDeviceService(cntrl)
	device.EXPECT().Delete(gomock.Eq("100"), gomock.Eq(true)).Times(1)
	s.PatchValue(&equinixClient, func(spec environscloudspec.CloudSpec) *packngo.Client {
		cl := &packngo.Client{}
		cl.Devices = device
		return cl
	})
	env, err := environs.Open(context.Background(), s.provider, environs.OpenParams{
		Cloud:  s.spec,
		Config: makeTestModelConfig(c),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.NotNil)
	err = env.StopInstances(envcontext.WithoutCredentialInvalidator(context.Background()), "100")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environProviderSuite) TestMapJujuSubnetsToReservationIDs(c *gc.C) {
	in := []map[network.Id][]string{
		{
			"subnet-abcdef-aa": []string{"fr"},
		},
		{
			"subnet-INFAN-42": []string{""},
		},
	}
	exp := []string{"abcdef-aa"}
	got := mapJujuSubnetsToReservationIDs(in)
	c.Assert(got, gc.DeepEquals, exp, gc.Commentf("expected FAN subnet to be filtered out"))
}

type packngoCreateDeviceMatcher struct {
	hostname, plan, metro, os, projectID string
}

func NewPackngoCreateDeviceMatcher(hostname, itype, metro, os, projectID string) gomock.Matcher {
	return &packngoCreateDeviceMatcher{hostname, itype, metro, os, projectID}
}

func (m *packngoCreateDeviceMatcher) String() string {
	return ""
}

func (m *packngoCreateDeviceMatcher) Matches(x interface{}) bool {
	req, ok := x.(*packngo.DeviceCreateRequest)
	if !ok {
		return false
	}
	if req.Hostname != m.hostname {
		return false
	}
	if req.Plan != m.plan {
		return false
	}
	if req.Metro != m.metro {
		return false
	}
	if req.ProjectID != m.projectID {
		return false
	}
	if req.OS != m.os {
		return false
	}
	return true
}

func (s *environProviderSuite) TestStartInstance(c *gc.C) {
	cntrl := gomock.NewController(c)
	defer cntrl.Finish()
	expDev := packngo.Device{
		ID:    "100",
		State: "active",
		Plan: &packngo.Plan{
			ID:          "18e285e0-1872-11ea-8d71-362b9e155667",
			Slug:        "g2.large.x86",
			Name:        "g2.large.x86",
			Description: "Our c3.small.x86 configuration is a zippy general use server, with a Intel Xeon E 2278G (8 cores, 16 threads) processor and 32GB of RAM.",
			Line:        "baremetal",
			Legacy:      true,
			Specs: &packngo.Specs{
				Cpus: []*packngo.Cpus{
					{
						Count: 1,
						Type:  "Intel(R) Xeon(R) E-2278G CPU @ 3.40GHz",
					},
				},
				Memory: &packngo.Memory{
					Total: "32GB",
				},
				Drives: []*packngo.Drives{
					{
						Count: 2,
						Size:  "480GB",
						Type:  "ssd",
					},
				},
			},
			Pricing: &packngo.Pricing{
				Hour: 0.5,
			},
			DeploymentTypes: []string{
				"on_demand",
				"spot_market",
			},
			Class: "g2.large.x86",
			AvailableInMetros: []packngo.Metro{
				{
					ID:      "108b2cfb-246b-45e3-885a-bf3e82fce1a0",
					Name:    "Amsterdam",
					Code:    "am",
					Country: "NL",
				},
			},
		},
	}
	device := mocks.NewMockDeviceService(cntrl)
	device.EXPECT().Create(NewPackngoCreateDeviceMatcher("juju-06f00d-0",
		"g2.large.x86",
		"am",
		"ubuntu_20_04",
		"12345c2a-6789-4d4f-a3c4-7367d6b7cca8")).Return(&expDev, nil, nil)

	device.EXPECT().Get(gomock.Eq("100"), nil).Return(&expDev, nil, nil)

	plan := mocks.NewMockPlanService(cntrl)
	opts := &packngo.ListOptions{
		Includes: []string{"available_in_metros"},
	}
	opts.Filter("line", "baremetal")
	opts.Filter("deployment_type", "on_demand")
	plan.EXPECT().List(opts).Return([]packngo.Plan{
		{
			ID:          "e818d69e-1ccf-11ec-9621-0242ac130002",
			Slug:        "test.baremetal-line-filtered.x86",
			Name:        "test.baremetal-line-filtered.x86",
			Description: "Our c3.small.x86 configuration is a zippy general use server, with a Intel Xeon E 2278G (8 cores, 16 threads) processor and 32GB of RAM.",
			Line:        "baremetal-line-filtered",
			Legacy:      true,
			Specs: &packngo.Specs{
				Cpus: []*packngo.Cpus{
					{
						Count: 1,
						Type:  "Intel(R) Xeon(R) E-2278G CPU @ 3.40GHz",
					},
				},
				Memory: &packngo.Memory{
					Total: "32GB",
				},
				Drives: []*packngo.Drives{
					{
						Count: 2,
						Size:  "480GB",
						Type:  "ssd",
					},
				},
			},
			Pricing: &packngo.Pricing{
				Hour: 0.5,
			},
			DeploymentTypes: []string{
				"on_demand",
			},
			Class: "test.baremetal-line-filtered.x86",
			AvailableInMetros: []packngo.Metro{
				{
					ID:      "108b2cfb-246b-45e3-885a-bf3e82fce1a0",
					Name:    "Amsterdam",
					Code:    "am",
					Country: "NL",
				},
			},
		},
		{
			ID:          "e818d69e-1ccf-11ec-9621-0242ac130002",
			Slug:        "test.deployment-type-filtered.x86",
			Name:        "test.deployment-type-filtered.x86",
			Description: "Our c3.small.x86 configuration is a zippy general use server, with a Intel Xeon E 2278G (8 cores, 16 threads) processor and 32GB of RAM.",
			Line:        "baremetal",
			Legacy:      true,
			Specs: &packngo.Specs{
				Cpus: []*packngo.Cpus{
					{
						Count: 1,
						Type:  "Intel(R) Xeon(R) E-2278G CPU @ 3.40GHz",
					},
				},
				Memory: &packngo.Memory{
					Total: "32GB",
				},
				Drives: []*packngo.Drives{
					{
						Count: 2,
						Size:  "480GB",
						Type:  "ssd",
					},
				},
			},
			Pricing: &packngo.Pricing{
				Hour: 0.5,
			},
			DeploymentTypes: []string{
				"deployment-type-filtered",
			},
			Class: "test.deployment-type-filtered.x86",
			AvailableInMetros: []packngo.Metro{
				{
					ID:      "108b2cfb-246b-45e3-885a-bf3e82fce1a0",
					Name:    "Amsterdam",
					Code:    "am",
					Country: "NL",
				},
			},
		},
		{
			ID:          "18e285e0-1872-11ea-8d71-362b9e155667",
			Slug:        "c3.small.x86",
			Name:        "c3.small.x86",
			Description: "Our c3.small.x86 configuration is a zippy general use server, with a Intel Xeon E 2278G (8 cores, 16 threads) processor and 32GB of RAM.",
			Line:        "baremetal",
			Legacy:      true,
			Specs: &packngo.Specs{
				Cpus: []*packngo.Cpus{
					{
						Count: 1,
						Type:  "Intel(R) Xeon(R) E-2278G CPU @ 3.40GHz",
					},
				},
				Memory: &packngo.Memory{
					Total: "32GB",
				},
				Drives: []*packngo.Drives{
					{
						Count: 2,
						Size:  "480GB",
						Type:  "ssd",
					},
				},
			},
			Pricing: &packngo.Pricing{
				Hour: 0.5,
			},
			DeploymentTypes: []string{
				"on_demand",
				"spot_market",
			},
			Class: "c3.small.x86",
			AvailableInMetros: []packngo.Metro{
				{
					ID:      "108b2cfb-246b-45e3-885a-bf3e82fce1a0",
					Name:    "Amsterdam",
					Code:    "am",
					Country: "NL",
				},
			},
		},
		{
			ID:          "18e285e0-1872-11ea-8d71-111111111111",
			Slug:        "g2.large.x86",
			Name:        "g2.large.x86",
			Description: "Our g2.large.x86 configuration is a zippy general use server, with a Intel Xeon E 2278G (8 cores, 16 threads) processor and 32GB of RAM.",
			Line:        "baremetal",
			Legacy:      true,
			Specs: &packngo.Specs{
				Cpus: []*packngo.Cpus{
					{
						Count: 1,
						Type:  "Intel(R) Xeon(R) E-2278G CPU @ 3.40GHz",
					},
				},
				Memory: &packngo.Memory{
					Total: "32GB",
				},
				Drives: []*packngo.Drives{
					{
						Count: 2,
						Size:  "480GB",
						Type:  "ssd",
					},
				},
			},
			Pricing: &packngo.Pricing{
				Hour: 1.20,
			},
			DeploymentTypes: []string{
				"on_demand",
				"spot_market",
			},
			Class: "c3.small.x86",
			AvailableInMetros: []packngo.Metro{
				{
					ID:      "108b2cfb-246b-45e3-885a-bf3e82fce1a0",
					Name:    "Amsterdam",
					Code:    "am",
					Country: "NL",
				},
			},
		},
		{
			ID:          "18e285e0-1872-11ea-8d71-362b9e155667",
			Slug:        "c3.large.arm",
			Name:        "c3.large.arm",
			Description: "c3.large.arm",
			Line:        "baremetal",
			Legacy:      true,
			Specs: &packngo.Specs{
				Cpus: []*packngo.Cpus{
					{
						Count: 1,
						Type:  "Ampere Altra Q80-30 80-core processor @ 2.8GHz",
					},
				},
				Memory: &packngo.Memory{
					Total: "32GB",
				},
				Drives: []*packngo.Drives{
					{
						Count: 2,
						Size:  "960GB",
						Type:  "NVME",
					},
				},
			},
			Pricing: &packngo.Pricing{
				Hour: 2,
			},
			DeploymentTypes: []string{
				"on_demand",
				"spot_market",
			},
			Class: "c3.large.arm",
			AvailableInMetros: []packngo.Metro{
				{
					ID:      "108b2cfb-246b-45e3-885a-bf3e82fce1a0",
					Name:    "Amsterdam",
					Code:    "am",
					Country: "NL",
				},
			},
		},
		{
			ID:          "351c618b-760f-45c5-b45d-7044111a6a31",
			Slug:        "nope.large.x86",
			Name:        "nope.large.x86",
			Description: "Tests that without on_demand this plan will not be returned",
			Line:        "baremetal",
			Legacy:      true,
			Specs: &packngo.Specs{
				Cpus: []*packngo.Cpus{
					{
						Count: 1,
						Type:  "Intel(R) Xeon(R) E-2278G CPU @ 3.40GHz",
					},
				},
				Memory: &packngo.Memory{
					Total: "32GB",
				},
				Drives: []*packngo.Drives{
					{
						Count: 2,
						Size:  "480GB",
						Type:  "ssd",
					},
				},
			},
			Pricing: &packngo.Pricing{
				Hour: 1.20,
			},
			DeploymentTypes: []string{},
			Class:           "nope.small.x86",
			AvailableInMetros: []packngo.Metro{
				{
					ID:      "108b2cfb-246b-45e3-885a-bf3e82fce1a0",
					Name:    "Amsterdam",
					Code:    "am",
					Country: "NL",
				},
			},
		},
	}, nil, nil)

	os := mocks.NewMockOSService(cntrl)
	os.EXPECT().List().Return([]packngo.OS{
		{
			Name:    "Ubuntu 18.04 LTS",
			Slug:    "ubuntu_18_04",
			Distro:  "ubuntu",
			Version: "18.04",
			ProvisionableOn: []string{
				"c1.large.arm",
				"c3.small.x86",
				"g2.large.x86",
			},
		},
		{
			Name:    "Ubuntu 20.04 LTS",
			Slug:    "ubuntu_20_04",
			Distro:  "ubuntu",
			Version: "20.04",
			ProvisionableOn: []string{
				"c1.large.arm",
				"c3.small.x86",
				"g2.large.x86",
			},
		},
	}, nil, nil)

	s.PatchValue(&equinixClient, func(spec environscloudspec.CloudSpec) *packngo.Client {
		cl := &packngo.Client{}
		cl.Devices = device
		cl.Plans = plan
		cl.OperatingSystems = os
		return cl
	})
	env, err := environs.Open(context.Background(), s.provider, environs.OpenParams{
		Cloud:  s.spec,
		Config: makeTestModelConfig(c),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.NotNil)
	cons := constraints.Value{}
	base := corebase.MakeDefaultBase("ubuntu", "20.04")
	iConfig, err := instancecfg.NewBootstrapInstanceConfig(testing.FakeControllerConfig(), cons, cons, base, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = env.StartInstance(envcontext.WithoutCredentialInvalidator(context.Background()), environs.StartInstanceParams{
		ControllerUUID:   env.Config().UUID(),
		AvailabilityZone: "yes",
		InstanceConfig:   iConfig,
		Constraints:      constraints.MustParse("instance-type=g2.large.x86"),
		Tools: tools.List{
			{
				Version: version.Binary{
					Arch: "amd64",
				},
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func makeTestModelConfig(c *gc.C, extra ...testing.Attrs) *config.Config {
	attrs := testing.Attrs{
		"type":          "equinix",
		"agent-version": "1.2.3",
	}
	for _, extra := range extra {
		attrs = attrs.Merge(extra)
	}
	attrs = testing.FakeConfig().Merge(attrs)
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

type EquinixUtils struct {
	jtesting.IsolationSuite
}

var _ = gc.Suite(&EquinixUtils{})

func (*EquinixUtils) TestWaitDeviceActive_ReturnProvisioning(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cl := &packngo.Client{}
	device := mocks.NewMockDeviceService(ctrl)
	device.EXPECT().Get(gomock.Eq("100"), nil).Return(&packngo.Device{
		ID:    "100",
		State: "provisioning",
	}, nil, nil).Times(1).Return(&packngo.Device{
		ID:    "100",
		State: "active",
	}, nil, nil).Times(1)
	cl.Devices = device
	_, err := waitDeviceActive(envcontext.WithoutCredentialInvalidator(context.Background()), cl, "100")

	c.Assert(err, jc.ErrorIsNil)
}

func (*EquinixUtils) TestWaitDeviceActive_ReturnActive(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cl := &packngo.Client{}
	device := mocks.NewMockDeviceService(ctrl)
	device.EXPECT().Get(gomock.Eq("100"), nil).Return(&packngo.Device{
		ID:    "100",
		State: "active",
	}, nil, nil).Times(1)
	cl.Devices = device
	_, err := waitDeviceActive(envcontext.WithoutCredentialInvalidator(context.Background()), cl, "100")
	c.Assert(err, jc.ErrorIsNil)
}

func (*EquinixUtils) TestWaitDeviceActive_ReturnFailed(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cl := &packngo.Client{}
	device := mocks.NewMockDeviceService(ctrl)
	device.EXPECT().Get(gomock.Eq("100"), nil).Return(&packngo.Device{
		ID:    "100",
		State: "failed",
	}, nil, nil).Times(1)
	cl.Devices = device
	_, err := waitDeviceActive(envcontext.WithoutCredentialInvalidator(context.Background()), cl, "100")
	c.Assert(err, gc.Not(jc.ErrorIsNil))
}

func (*EquinixUtils) TestIsDistroSupported(c *gc.C) {
	for _, s := range []struct {
		os     packngo.OS
		ic     *instances.InstanceConstraint
		expect bool
	}{
		{
			os: packngo.OS{
				Name:    "ubuntu",
				Version: "20.10",
			},
			ic: &instances.InstanceConstraint{
				Base: corebase.MakeDefaultBase("ubuntu", "20.10"),
			},
			expect: false,
		},
	} {
		o := isDistroSupported(s.os, s.ic)
		if o != s.expect {
			c.Errorf("for os \"%s\" expected \"%+v\" got \"%+v\"", s.os.Name, s.expect, o)
		}
	}
}

func (*EquinixUtils) TestGetArchitectureFromPlan(c *gc.C) {
	for _, s := range []struct {
		plan   string
		expect string
	}{
		{
			plan:   "metal.medium.x86",
			expect: "amd64",
		},
		{
			plan:   "c2.small.arm",
			expect: "arm64",
		},
		{
			plan:   "default",
			expect: "amd64",
		},
	} {
		o := getArchitectureFromPlan(s.plan)
		if o != s.expect {
			c.Errorf("for plan \"%s\" expected \"%s\" got \"%s\"", s.plan, s.expect, o)
		}
	}
}

func (*EquinixUtils) TestValidPlan(c *gc.C) {
	const UNEXPECTED = "unexpected"

	plan := func(f func(*packngo.Plan)) packngo.Plan {
		p := &packngo.Plan{
			Slug:            "test",
			Name:            "test.x86",
			Line:            "baremetal",
			DeploymentTypes: []string{"on_demand"},
			Pricing:         &packngo.Pricing{},
			Specs: &packngo.Specs{
				Memory: &packngo.Memory{Total: "32GB"},
				Cpus:   []*packngo.Cpus{{Count: 1}},
			},
			AvailableInMetros: []packngo.Metro{{Code: "dc"}},
		}
		f(p)
		return *p
	}
	for _, s := range []struct {
		name   string
		plan   packngo.Plan
		region string
		expect bool
	}{
		{
			name:   "matched",
			plan:   plan(func(p *packngo.Plan) {}),
			region: "dc",
			expect: true,
		},
		{
			name: "unexpected.line",
			plan: plan(func(p *packngo.Plan) {
				p.Line = UNEXPECTED
			}),
			region: "dc",
			expect: false,
		},
		{
			plan: plan(func(p *packngo.Plan) {
				p.Slug = "unexpected.deploymenttype"
				p.DeploymentTypes[0] = UNEXPECTED
			}),
			region: "dc",
			expect: false,
		},
		{
			plan: plan(func(p *packngo.Plan) {
				p.Slug = "unexpected.metro"
				p.AvailableInMetros[0].Code = UNEXPECTED
			}),
			region: "dc",
			expect: false,
		},
	} {
		o := validPlan(s.plan, s.region)
		if o != s.expect {
			c.Errorf("for plan \"%s\" expected \"%s\" got \"%s\"", s.name, s.expect, o)
		}
	}
}
