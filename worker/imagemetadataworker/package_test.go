// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadataworker_test

import (
	"testing"

	gc "gopkg.in/check.v1"

	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/imagemetadata"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/dummy"
	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseMetadataSuite struct {
	coretesting.BaseSuite
}

func (s *baseMetadataSuite) ImageClient(save func(m []params.CloudImageMetadata) error) *imagemetadata.Client {
	closer := apitesting.APICallerFunc(func(objType string, version int, id, request string, a, result interface{}) error {
		if request == "List" {
			panic("worker should never call api list")
		}
		args, ok := a.(params.MetadataSaveParams)
		if !ok {
			panic("wrong arguments passed to mock")
		}

		if request == "Save" {
			return save(args.Metadata)
		}
		return nil
	})

	return imagemetadata.NewClient(closer)
}

func (s *baseMetadataSuite) SomeEnviron() environs.Environ {
	return anEnv{}
}

type anEnv struct {
	environs.Environ
}

func (e anEnv) Config() *config.Config {
	attrs := dummy.SampleConfig().Merge(coretesting.Attrs{
		"type": "nonex",
	})
	cfg, _ := config.New(config.NoDefaults, attrs)
	return cfg
}
