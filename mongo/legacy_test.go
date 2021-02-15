// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo_test

import (
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/service/common"
	svctesting "github.com/juju/juju/service/common/testing"
	coretesting "github.com/juju/juju/testing"
)

type LegacySuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&LegacySuite{})

func (s *LegacySuite) TestMaybeUseLegacyMongo(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ostools := mongo.NewMockSearchTools(ctrl)

	dataDir := c.MkDir()
	args := mongo.EnsureServerParams{
		DataDir:   dataDir,
		OplogSize: 10,
	}

	ostools.EXPECT().Exists("/usr/bin/mongod").Return(false)
	ostools.EXPECT().Exists("/usr/lib/juju/mongo3.2/bin/mongod").Return(true)
	ostools.EXPECT().GetCommandOutput("/usr/lib/juju/mongo3.2/bin/mongod", "--version").Return(
		"db version v3.2.0", nil)

	data := svctesting.NewFakeServiceData()
	err := data.SetStatus("juju-db", "installed")
	c.Assert(err, jc.ErrorIsNil)
	err = data.SetStatus("juju-db", "running")
	c.Assert(err, jc.ErrorIsNil)

	s.PatchValue(mongo.NewService, func(name string, conf common.Conf) (mongo.MongoService, error) {
		svc := svctesting.NewFakeService(name, conf)
		svc.FakeServiceData = data
		return svc, nil
	})
	err = mongo.MaybeUseLegacyMongo(args, ostools)
	c.Assert(err, jc.ErrorIsNil)
}
