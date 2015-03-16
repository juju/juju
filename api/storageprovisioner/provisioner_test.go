// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	"errors"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/storageprovisioner"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&provisionerSuite{})

type provisionerSuite struct {
	coretesting.BaseSuite
}

func (s *provisionerSuite) TestWatchVolumes(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchVolumes")
		c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})

	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	_, err := st.WatchVolumes()
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *provisionerSuite) TestWatchVolumeAttachments(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchVolumeAttachments")
		c.Assert(result, gc.FitsTypeOf, &params.MachineStorageIdsWatchResults{})
		*(result.(*params.MachineStorageIdsWatchResults)) = params.MachineStorageIdsWatchResults{
			Results: []params.MachineStorageIdsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})

	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	_, err := st.WatchVolumeAttachments()
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *provisionerSuite) TestVolumes(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "Volumes")
		c.Check(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{"volume-100"}}})
		c.Assert(result, gc.FitsTypeOf, &params.VolumeResults{})
		*(result.(*params.VolumeResults)) = params.VolumeResults{
			Results: []params.VolumeResult{{
				Result: params.Volume{
					VolumeTag: "volume-100",
					VolumeId:  "volume-id",
					Serial:    "abc",
					Size:      1024,
				},
			}},
		}
		callCount++
		return nil
	})

	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	volumes, err := st.Volumes([]names.VolumeTag{names.NewVolumeTag("100")})
	c.Check(err, jc.ErrorIsNil)
	c.Check(callCount, gc.Equals, 1)
	c.Assert(volumes, jc.DeepEquals, []params.VolumeResult{{
		Result: params.Volume{
			VolumeTag: "volume-100", VolumeId: "volume-id", Serial: "abc", Size: 1024,
		},
	}})
}

func (s *provisionerSuite) TestVolumeParams(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "VolumeParams")
		c.Check(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{"volume-100"}}})
		c.Assert(result, gc.FitsTypeOf, &params.VolumeParamsResults{})
		*(result.(*params.VolumeParamsResults)) = params.VolumeParamsResults{
			Results: []params.VolumeParamsResult{{
				Result: params.VolumeParams{
					VolumeTag:  "volume-100",
					Size:       1024,
					Provider:   "loop",
					MachineTag: "machine-200",
				},
			}},
		}
		callCount++
		return nil
	})

	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	volumeParams, err := st.VolumeParams([]names.VolumeTag{names.NewVolumeTag("100")})
	c.Check(err, jc.ErrorIsNil)
	c.Check(callCount, gc.Equals, 1)
	c.Assert(volumeParams, jc.DeepEquals, []params.VolumeParamsResult{{
		Result: params.VolumeParams{
			VolumeTag: "volume-100", Size: 1024, Provider: "loop", MachineTag: "machine-200",
		},
	}})
}

func (s *provisionerSuite) TestSetVolumeInfo(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "SetVolumeInfo")
		c.Check(arg, gc.DeepEquals, params.Volumes{
			Volumes: []params.Volume{{VolumeTag: "volume-100", VolumeId: "123", Serial: "abc", Size: 1024}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: nil}},
		}
		callCount++
		return nil
	})

	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	volumes := []params.Volume{{VolumeTag: "volume-100", VolumeId: "123", Serial: "abc", Size: 1024}}
	errorResults, err := st.SetVolumeInfo(volumes)
	c.Check(err, jc.ErrorIsNil)
	c.Check(callCount, gc.Equals, 1)
	c.Assert(errorResults.OneError(), jc.ErrorIsNil)
}

func (s *provisionerSuite) testOpWithTags(
	c *gc.C, opName string, apiCall func(*storageprovisioner.State, []names.Tag) ([]params.ErrorResult, error),
) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, opName)
		c.Check(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "volume-100"}}})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: nil}},
		}
		callCount++
		return nil
	})

	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	volumes := []names.Tag{names.NewVolumeTag("100")}
	errorResults, err := apiCall(st, volumes)
	c.Check(err, jc.ErrorIsNil)
	c.Check(callCount, gc.Equals, 1)
	c.Assert(errorResults, jc.DeepEquals, []params.ErrorResult{{}})
}

func (s *provisionerSuite) TestRemove(c *gc.C) {
	s.testOpWithTags(c, "Remove", func(st *storageprovisioner.State, tags []names.Tag) ([]params.ErrorResult, error) {
		return st.Remove(tags)
	})
}

func (s *provisionerSuite) TestEnsureDead(c *gc.C) {
	s.testOpWithTags(c, "EnsureDead", func(st *storageprovisioner.State, tags []names.Tag) ([]params.ErrorResult, error) {
		return st.EnsureDead(tags)
	})
}

func (s *provisionerSuite) TestLife(c *gc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "StorageProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "Life")
		c.Check(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "volume-100"}}})
		c.Assert(result, gc.FitsTypeOf, &params.LifeResults{})
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{Life: params.Alive}},
		}
		callCount++
		return nil
	})

	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	volumes := []names.Tag{names.NewVolumeTag("100")}
	lifeResults, err := st.Life(volumes)
	c.Check(err, jc.ErrorIsNil)
	c.Check(callCount, gc.Equals, 1)
	c.Assert(lifeResults, jc.DeepEquals, []params.LifeResult{{Life: params.Alive}})
}

func (s *provisionerSuite) testClientError(c *gc.C, apiCall func(*storageprovisioner.State) error) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return errors.New("blargh")
	})
	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	err := apiCall(st)
	c.Check(err, gc.ErrorMatches, "blargh")
}

func (s *provisionerSuite) TestWatchVolumesClientError(c *gc.C) {
	s.testClientError(c, func(st *storageprovisioner.State) error {
		_, err := st.WatchVolumes()
		return err
	})
}

func (s *provisionerSuite) TestVolumesClientError(c *gc.C) {
	s.testClientError(c, func(st *storageprovisioner.State) error {
		_, err := st.Volumes(nil)
		return err
	})
}
func (s *provisionerSuite) TestVolumeParamsClientError(c *gc.C) {
	s.testClientError(c, func(st *storageprovisioner.State) error {
		_, err := st.VolumeParams(nil)
		return err
	})
}
func (s *provisionerSuite) TestRemoveClientError(c *gc.C) {
	s.testClientError(c, func(st *storageprovisioner.State) error {
		_, err := st.Remove(nil)
		return err
	})
}
func (s *provisionerSuite) TestSetVolumeInfoClientError(c *gc.C) {
	s.testClientError(c, func(st *storageprovisioner.State) error {
		_, err := st.SetVolumeInfo(nil)
		return err
	})
}

func (s *provisionerSuite) TestEnsureDeadClientError(c *gc.C) {
	s.testClientError(c, func(st *storageprovisioner.State) error {
		_, err := st.EnsureDead(nil)
		return err
	})
}

func (s *provisionerSuite) TestLifeClientError(c *gc.C) {
	s.testClientError(c, func(st *storageprovisioner.State) error {
		_, err := st.Life(nil)
		return err
	})
}

func (s *provisionerSuite) TestWatchVolumesServerError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: &params.Error{Message: "MSG", Code: "621"},
			}},
		}
		return nil
	})
	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	_, err := st.WatchVolumes()
	c.Check(err, gc.ErrorMatches, "MSG")
}

func (s *provisionerSuite) TestVolumesServerError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.VolumeResults)) = params.VolumeResults{
			Results: []params.VolumeResult{{
				Error: &params.Error{Message: "MSG", Code: "621"},
			}},
		}
		return nil
	})
	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	results, err := st.Volumes([]names.VolumeTag{names.NewVolumeTag("100")})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Check(results[0].Error, gc.ErrorMatches, "MSG")
}
func (s *provisionerSuite) TestVolumeParamsServerError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.VolumeParamsResults)) = params.VolumeParamsResults{
			Results: []params.VolumeParamsResult{{
				Error: &params.Error{Message: "MSG", Code: "621"},
			}},
		}
		return nil
	})
	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	results, err := st.VolumeParams([]names.VolumeTag{names.NewVolumeTag("100")})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Check(results[0].Error, gc.ErrorMatches, "MSG")
}

func (s *provisionerSuite) TestSetVolumeInfoServerError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "MSG", Code: "621"},
			}},
		}
		return nil
	})
	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	results, err := st.SetVolumeInfo([]params.Volume{{
		VolumeTag: names.NewVolumeTag("100").String(),
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Check(results.OneError(), gc.ErrorMatches, "MSG")
}

func (s *provisionerSuite) testServerError(c *gc.C, apiCall func(*storageprovisioner.State, []names.Tag) ([]params.ErrorResult, error)) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "MSG", Code: "621"},
			}},
		}
		return nil
	})
	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	tags := []names.Tag{
		names.NewVolumeTag("100"),
	}
	results, err := apiCall(st, tags)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Check(results[0].Error, gc.ErrorMatches, "MSG")
}

func (s *provisionerSuite) TestRemoveServerError(c *gc.C) {
	s.testServerError(c, func(st *storageprovisioner.State, tags []names.Tag) ([]params.ErrorResult, error) {
		return st.Remove(tags)
	})
}

func (s *provisionerSuite) TestEnsureDeadServerError(c *gc.C) {
	s.testServerError(c, func(st *storageprovisioner.State, tags []names.Tag) ([]params.ErrorResult, error) {
		return st.EnsureDead(tags)
	})
}

func (s *provisionerSuite) TestLifeServerError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{
				Error: &params.Error{Message: "MSG", Code: "621"},
			}},
		}
		return nil
	})
	st := storageprovisioner.NewState(apiCaller, names.NewMachineTag("123"))
	tags := []names.Tag{
		names.NewVolumeTag("100"),
	}
	results, err := st.Life(tags)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Check(results[0].Error, gc.ErrorMatches, "MSG")
}
