// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/juju/errors"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/servicefactory"
)

func (s *workerSuite) TestFailRetrievingSpaces(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := bootstrapWorker{
		cfg: WorkerConfig{
			SpaceService:  s.spaceService,
			SubnetService: s.subnetService,
		},
	}

	s.spaceService.EXPECT().GetAllSpaces(gomock.Any()).Return(nil, errors.New("boom"))

	err := w.ensureInitialModelSpaces(context.Background(), "", nil)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *workerSuite) TestFailInsertSubnets(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := bootstrapWorker{
		cfg: WorkerConfig{
			SpaceService:         s.spaceService,
			SubnetService:        s.subnetService,
			ServiceFactoryGetter: s.serviceFactoryGetter,
			SpaceServiceGetter: func(getter servicefactory.ServiceFactory) SpaceService {
				return s.initialModelSpaceService
			},
			SubnetServiceGetter: func(getter servicefactory.ServiceFactory) SubnetService {
				return s.initialModelSubnetService
			},
		},
	}

	spaces := network.SpaceInfos{
		{
			Name:       "space-0",
			ID:         "space0",
			ProviderId: "provider0",
			Subnets: network.SubnetInfos{
				{
					ID:        "subnet0",
					SpaceID:   "space0",
					SpaceName: "space-0",
					CIDR:      "10.0.0.0/24",
				},
			},
		},
	}
	s.spaceService.EXPECT().GetAllSpaces(gomock.Any()).Return(spaces, nil)
	s.serviceFactoryGetter.EXPECT().FactoryForModel("initial-model-uuid")
	s.initialModelSubnetService.EXPECT().
		AddSubnet(gomock.Any(), network.SubnetInfo{
			// ID must be "" when inserting the copied subnet
			ID:        "",
			SpaceID:   "",
			SpaceName: "space-0",
			CIDR:      "10.0.0.0/24",
		}).
		Return(network.Id(""), errors.New("inserting subnet"))

	err := w.ensureInitialModelSpaces(context.Background(), "initial-model-uuid", nil)
	c.Assert(err, gc.ErrorMatches, "inserting subnet")
}

func (s *workerSuite) TestFailInsertSpaces(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := bootstrapWorker{
		cfg: WorkerConfig{
			SpaceService:         s.spaceService,
			SubnetService:        s.subnetService,
			ServiceFactoryGetter: s.serviceFactoryGetter,
			SpaceServiceGetter: func(getter servicefactory.ServiceFactory) SpaceService {
				return s.initialModelSpaceService
			},
			SubnetServiceGetter: func(getter servicefactory.ServiceFactory) SubnetService {
				return s.initialModelSubnetService
			},
		},
	}

	spaces := network.SpaceInfos{
		{
			Name:       "space-0",
			ID:         "space0",
			ProviderId: "provider0",
			Subnets: network.SubnetInfos{
				{
					ID:        "subnet0",
					SpaceID:   "space0",
					SpaceName: "space-0",
					CIDR:      "10.0.0.0/24",
				},
			},
		},
	}
	s.spaceService.EXPECT().GetAllSpaces(gomock.Any()).Return(spaces, nil)
	s.serviceFactoryGetter.EXPECT().FactoryForModel("initial-model-uuid")
	s.initialModelSubnetService.EXPECT().
		AddSubnet(gomock.Any(), network.SubnetInfo{
			// ID must be "" when inserting the copied subnet
			ID:        "",
			SpaceID:   "",
			SpaceName: "space-0",
			CIDR:      "10.0.0.0/24",
		}).
		Return(network.Id("subnet1"), nil)
	s.initialModelSpaceService.EXPECT().AddSpace(gomock.Any(), "space-0", network.Id("provider0"), []string{"subnet1"}).
		Return(network.Id(""), errors.New("inserting space"))

	err := w.ensureInitialModelSpaces(context.Background(), "initial-model-uuid", nil)
	c.Assert(err, gc.ErrorMatches, "inserting space")
}

func (s *workerSuite) TestInsertCopiedSpaces(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := bootstrapWorker{
		cfg: WorkerConfig{
			SpaceService:         s.spaceService,
			SubnetService:        s.subnetService,
			ServiceFactoryGetter: s.serviceFactoryGetter,
			SpaceServiceGetter: func(getter servicefactory.ServiceFactory) SpaceService {
				return s.initialModelSpaceService
			},
			SubnetServiceGetter: func(getter servicefactory.ServiceFactory) SubnetService {
				return s.initialModelSubnetService
			},
		},
	}

	spaces := network.SpaceInfos{
		{
			Name:       "space-0",
			ID:         "space0",
			ProviderId: "provider0",
			Subnets: network.SubnetInfos{
				{
					ID:        "subnet0",
					SpaceID:   "space0",
					SpaceName: "space-0",
					CIDR:      "10.0.0.0/24",
				},
				{
					ID:        "subnet1",
					SpaceID:   "space0",
					SpaceName: "space-0",
					CIDR:      "10.0.1.0/24",
				},
			},
		},
		{
			Name:       "space-1",
			ID:         "space1",
			ProviderId: "provider1",
			Subnets: network.SubnetInfos{
				{
					ID:        "subnet2",
					SpaceID:   "space1",
					SpaceName: "space-1",
					CIDR:      "10.0.2.0/24",
				},
				{
					ID:        "subnet3",
					SpaceID:   "space1",
					SpaceName: "space-1",
					CIDR:      "10.0.3.0/24",
				},
			},
		},
	}
	s.spaceService.EXPECT().GetAllSpaces(gomock.Any()).Return(spaces, nil)
	s.serviceFactoryGetter.EXPECT().FactoryForModel("initial-model-uuid")
	// First space
	s.initialModelSubnetService.EXPECT().
		AddSubnet(gomock.Any(), network.SubnetInfo{
			// ID must be "" when inserting the copied subnet
			ID:        "",
			SpaceID:   "",
			SpaceName: "space-0",
			CIDR:      "10.0.0.0/24",
		}).
		Return(network.Id("subnet10"), nil)
	s.initialModelSubnetService.EXPECT().
		AddSubnet(gomock.Any(), network.SubnetInfo{
			// ID must be "" when inserting the copied subnet
			ID:        "",
			SpaceID:   "",
			SpaceName: "space-0",
			CIDR:      "10.0.1.0/24",
		}).
		Return(network.Id("subnet11"), nil)
	s.initialModelSpaceService.EXPECT().AddSpace(gomock.Any(), "space-0", network.Id("provider0"), []string{"subnet10", "subnet11"}).
		Return(network.Id("space10"), nil)
	s.initialModelSubnetService.EXPECT().UpdateSubnet(gomock.Any(), "subnet10", "space10")
	s.initialModelSubnetService.EXPECT().UpdateSubnet(gomock.Any(), "subnet11", "space10")
	// Second space
	s.initialModelSubnetService.EXPECT().
		AddSubnet(gomock.Any(), network.SubnetInfo{
			// ID must be "" when inserting the copied subnet
			ID:        "",
			SpaceID:   "",
			SpaceName: "space-1",
			CIDR:      "10.0.2.0/24",
		}).
		Return(network.Id("subnet12"), nil)
	s.initialModelSubnetService.EXPECT().
		AddSubnet(gomock.Any(), network.SubnetInfo{
			// ID must be "" when inserting the copied subnet
			ID:        "",
			SpaceID:   "",
			SpaceName: "space-1",
			CIDR:      "10.0.3.0/24",
		}).
		Return(network.Id("subnet13"), nil)
	s.initialModelSpaceService.EXPECT().AddSpace(gomock.Any(), "space-1", network.Id("provider1"), []string{"subnet12", "subnet13"}).
		Return(network.Id("space11"), nil)
	s.initialModelSubnetService.EXPECT().UpdateSubnet(gomock.Any(), "subnet12", "space11")
	s.initialModelSubnetService.EXPECT().UpdateSubnet(gomock.Any(), "subnet13", "space11")

	err := w.ensureInitialModelSpaces(context.Background(), "initial-model-uuid", nil)
	c.Assert(err, gc.IsNil)
}
