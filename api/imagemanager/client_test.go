// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemanager_test

import (
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/imagemanager"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
)

type imagemanagerSuite struct {
	coretesting.BaseSuite

	imagemanager *imagemanager.Client
}

var _ = gc.Suite(&imagemanagerSuite{})

var dummyTime = time.Now()

func (s *imagemanagerSuite) TestListImages(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "ImageManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "ListImages")
		c.Check(arg, gc.DeepEquals, params.ImageFilterParams{
			Images: []params.ImageSpec{{
				Kind:   "lxc",
				Series: "trusty",
				Arch:   "amd64",
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ListImageResult{})
		*(result.(*params.ListImageResult)) = params.ListImageResult{
			Result: []params.ImageMetadata{{
				Kind:    "lxc",
				Series:  "trusty",
				Arch:    "amd64",
				Created: dummyTime,
				URL:     "http://path",
			}},
		}
		callCount++
		return nil
	})

	im := imagemanager.NewClient(apiCaller)
	imageMetadata, err := im.ListImages("lxc", "trusty", "amd64")
	c.Check(err, jc.ErrorIsNil)
	c.Check(callCount, gc.Equals, 1)
	c.Check(imageMetadata, gc.HasLen, 1)
	metadata := imageMetadata[0]
	c.Assert(metadata, gc.Equals, params.ImageMetadata{
		Kind:    "lxc",
		Series:  "trusty",
		Arch:    "amd64",
		Created: dummyTime,
		URL:     "http://path",
	})
}

func (s *imagemanagerSuite) TestDeleteImage(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "ImageManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "DeleteImages")
		c.Check(arg, gc.DeepEquals, params.ImageFilterParams{
			Images: []params.ImageSpec{{
				Kind:   "lxc",
				Series: "trusty",
				Arch:   "amd64",
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: nil,
			}},
		}
		callCount++
		return nil
	})

	im := imagemanager.NewClient(apiCaller)
	err := im.DeleteImage("lxc", "trusty", "amd64")
	c.Check(err, jc.ErrorIsNil)
	c.Check(callCount, gc.Equals, 1)
}

func (s *imagemanagerSuite) TestDeleteImageCallError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return errors.New("an error")
	})

	im := imagemanager.NewClient(apiCaller)
	err := im.DeleteImage("lxc", "trusty", "amd64")
	c.Check(err, gc.ErrorMatches, "an error")
}

func (s *imagemanagerSuite) TestDeleteImageError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "the devil made me do it", Code: "666"},
			}},
		}
		return nil
	})

	im := imagemanager.NewClient(apiCaller)
	err := im.DeleteImage("lxc", "trusty", "amd64")
	c.Check(err, gc.ErrorMatches, "the devil made me do it")
}
