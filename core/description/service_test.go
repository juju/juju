// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"
)

type ServiceSerializationSuite struct {
	SliceSerializationSuite
	StatusHistoryMixinSuite
}

var _ = gc.Suite(&ServiceSerializationSuite{})

func (s *ServiceSerializationSuite) SetUpTest(c *gc.C) {
	s.SliceSerializationSuite.SetUpTest(c)
	s.importName = "services"
	s.sliceName = "services"
	s.importFunc = func(m map[string]interface{}) (interface{}, error) {
		return importServices(m)
	}
	s.testFields = func(m map[string]interface{}) {
		m["services"] = []interface{}{}
	}
	s.StatusHistoryMixinSuite.creator = func() HasStatusHistory {
		return minimalService()
	}
	s.StatusHistoryMixinSuite.serializer = func(c *gc.C, initial interface{}) HasStatusHistory {
		return s.exportImport(c, initial.(*service))
	}
}

func minimalServiceMap() map[interface{}]interface{} {
	return map[interface{}]interface{}{
		"name":           "ubuntu",
		"series":         "trusty",
		"charm-url":      "cs:trusty/ubuntu",
		"status":         minimalStatusMap(),
		"status-history": emptyStatusHistoryMap(),
		"settings": map[interface{}]interface{}{
			"key": "value",
		},
		"settings-refcount": 1,
		"leadership-settings": map[interface{}]interface{}{
			"leader": true,
		},
		"metrics-creds": "c2Vrcml0", // base64 encoded
		"units": map[interface{}]interface{}{
			"version": 1,
			"units": []interface{}{
				minimalUnitMap(),
			},
		},
	}
}

func minimalService() *service {
	s := newService(minimalServiceArgs())
	s.SetStatus(minimalStatusArgs())
	u := s.AddUnit(minimalUnitArgs())
	u.SetAgentStatus(minimalStatusArgs())
	u.SetWorkloadStatus(minimalStatusArgs())
	u.SetTools(minimalAgentToolsArgs())
	return s
}

func addMinimalService(model Model) {
	s := model.AddService(minimalServiceArgs())
	s.SetStatus(minimalStatusArgs())
	u := s.AddUnit(minimalUnitArgs())
	u.SetAgentStatus(minimalStatusArgs())
	u.SetWorkloadStatus(minimalStatusArgs())
	u.SetTools(minimalAgentToolsArgs())
}

func minimalServiceArgs() ServiceArgs {
	return ServiceArgs{
		Tag:      names.NewServiceTag("ubuntu"),
		Series:   "trusty",
		CharmURL: "cs:trusty/ubuntu",
		Settings: map[string]interface{}{
			"key": "value",
		},
		SettingsRefCount: 1,
		LeadershipSettings: map[string]interface{}{
			"leader": true,
		},
		MetricsCredentials: []byte("sekrit"),
	}
}

func (s *ServiceSerializationSuite) TestNewService(c *gc.C) {
	args := ServiceArgs{
		Tag:         names.NewServiceTag("magic"),
		Series:      "zesty",
		Subordinate: true,
		CharmURL:    "cs:zesty/magic",
		ForceCharm:  true,
		Exposed:     true,
		MinUnits:    42, // no judgement is made by the migration code
		Settings: map[string]interface{}{
			"key": "value",
		},
		SettingsRefCount: 1,
		LeadershipSettings: map[string]interface{}{
			"leader": true,
		},
		MetricsCredentials: []byte("sekrit"),
	}
	service := newService(args)

	c.Assert(service.Name(), gc.Equals, "magic")
	c.Assert(service.Tag(), gc.Equals, names.NewServiceTag("magic"))
	c.Assert(service.Series(), gc.Equals, "zesty")
	c.Assert(service.Subordinate(), jc.IsTrue)
	c.Assert(service.CharmURL(), gc.Equals, "cs:zesty/magic")
	c.Assert(service.ForceCharm(), jc.IsTrue)
	c.Assert(service.Exposed(), jc.IsTrue)
	c.Assert(service.MinUnits(), gc.Equals, 42)
	c.Assert(service.Settings(), jc.DeepEquals, args.Settings)
	c.Assert(service.SettingsRefCount(), gc.Equals, 1)
	c.Assert(service.LeadershipSettings(), jc.DeepEquals, args.LeadershipSettings)
	c.Assert(service.MetricsCredentials(), jc.DeepEquals, []byte("sekrit"))
}

func (s *ServiceSerializationSuite) TestMinimalServiceValid(c *gc.C) {
	service := minimalService()
	c.Assert(service.Validate(), jc.ErrorIsNil)
}

func (s *ServiceSerializationSuite) TestMinimalMatches(c *gc.C) {
	bytes, err := yaml.Marshal(minimalService())
	c.Assert(err, jc.ErrorIsNil)

	var source map[interface{}]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(source, jc.DeepEquals, minimalServiceMap())
}

func (s *ServiceSerializationSuite) exportImport(c *gc.C, service_ *service) *service {
	initial := services{
		Version:   1,
		Services_: []*service{service_},
	}

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	services, err := importServices(source)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(services, gc.HasLen, 1)
	return services[0]
}

func (s *ServiceSerializationSuite) TestParsingSerializedData(c *gc.C) {
	svc := minimalService()
	service := s.exportImport(c, svc)
	c.Assert(service, jc.DeepEquals, svc)
}

func (s *ServiceSerializationSuite) TestAnnotations(c *gc.C) {
	initial := minimalService()
	annotations := map[string]string{
		"string":  "value",
		"another": "one",
	}
	initial.SetAnnotations(annotations)

	service := s.exportImport(c, initial)
	c.Assert(service.Annotations(), jc.DeepEquals, annotations)
}

func (s *ServiceSerializationSuite) TestConstraints(c *gc.C) {
	initial := minimalService()
	args := ConstraintsArgs{
		Architecture: "amd64",
		Memory:       8 * gig,
		RootDisk:     40 * gig,
	}
	initial.SetConstraints(args)

	service := s.exportImport(c, initial)
	c.Assert(service.Constraints(), jc.DeepEquals, newConstraints(args))
}
