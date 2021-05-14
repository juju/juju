// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package equinix

import (
	"net/http"

	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"github.com/packethost/packngo"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/provider/equinix/mocks"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	jtesting "github.com/juju/testing"
)

type environProviderSuite struct {
	jtesting.IsolationSuite
	provider environs.EnvironProvider
	spec     environscloudspec.CloudSpec
	requests []*http.Request
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
	env, err := environs.Open(s.provider, environs.OpenParams{
		Cloud:  s.spec,
		Config: makeTestModelConfig(c),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.NotNil)
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
	env, err := environs.Open(s.provider, environs.OpenParams{
		Cloud:  s.spec,
		Config: makeTestModelConfig(c),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.NotNil)
	err = env.StopInstances(context.NewCloudCallContext(), "100")
	c.Assert(err, jc.ErrorIsNil)
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
	plan.EXPECT().List(&packngo.ListOptions{
		Includes: []string{"available_in_metros"},
	}).Return([]packngo.Plan{
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
	env, err := environs.Open(s.provider, environs.OpenParams{
		Cloud:  s.spec,
		Config: makeTestModelConfig(c),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.NotNil)
	cons := constraints.Value{}
	iConfig, err := instancecfg.NewBootstrapInstanceConfig(testing.FakeControllerConfig(), cons, cons, "focal", "", nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = env.StartInstance(context.NewCloudCallContext(), environs.StartInstanceParams{
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

func (s *environProviderSuite) testOpenError(c *gc.C, spec environscloudspec.CloudSpec, expect string) {
	_, err := environs.Open(s.provider, environs.OpenParams{
		Cloud:  spec,
		Config: makeTestModelConfig(c),
	})
	c.Assert(err, gc.ErrorMatches, expect)
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
	_, err := waitDeviceActive(context.NewCloudCallContext(), cl, "100")

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
	_, err := waitDeviceActive(context.NewCloudCallContext(), cl, "100")

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
	waitDeviceActive(context.NewCloudCallContext(), cl, "100")
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
				Series: "20.10",
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
