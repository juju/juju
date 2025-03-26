// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	"gopkg.in/httprequest.v1"

	"github.com/juju/juju/api/client/modelupgrader"
	"github.com/juju/juju/api/client/modelupgrader/mocks"
	coretesting "github.com/juju/juju/internal/testing"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/internal/version"
	"github.com/juju/juju/rpc/params"
)

type UpgradeModelSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&UpgradeModelSuite{})

func (s *UpgradeModelSuite) TestAbortModelUpgrade(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	apiCaller := mocks.NewMockAPICallCloser(ctrl)

	apiCaller.EXPECT().BestFacadeVersion("ModelUpgrader").Return(1)
	apiCaller.EXPECT().APICall(
		gomock.Any(),
		"ModelUpgrader", 1, "", "AbortModelUpgrade",
		params.ModelParam{
			ModelTag: coretesting.ModelTag.String(),
		}, nil,
	).Return(nil)

	client := modelupgrader.NewClient(apiCaller)
	err := client.AbortModelUpgrade(context.Background(), coretesting.ModelTag.Id())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UpgradeModelSuite) TestUpgradeModel(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	apiCaller := mocks.NewMockAPICallCloser(ctrl)

	apiCaller.EXPECT().BestFacadeVersion("ModelUpgrader").Return(1)
	apiCaller.EXPECT().APICall(
		gomock.Any(),
		"ModelUpgrader", 1, "", "UpgradeModel",
		params.UpgradeModelParams{
			ModelTag:            coretesting.ModelTag.String(),
			TargetVersion:       version.MustParse("2.9.1"),
			IgnoreAgentVersions: true,
			DryRun:              true,
		}, &params.UpgradeModelResult{},
	).DoAndReturn(func(ctx context.Context, objType string, facadeVersion int, id, request string, args, result interface{}) error {
		out := result.(*params.UpgradeModelResult)
		out.ChosenVersion = version.MustParse("2.9.99")
		return nil
	})

	client := modelupgrader.NewClient(apiCaller)
	chosenVersion, err := client.UpgradeModel(
		context.Background(),
		coretesting.ModelTag.Id(),
		version.MustParse("2.9.1"),
		"", true, true,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(chosenVersion, gc.DeepEquals, version.MustParse("2.9.99"))
}

func (s *UpgradeModelSuite) TestUploadTools(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	apiCaller := mocks.NewMockAPICallCloser(ctrl)
	doer := mocks.NewMockDoer(ctrl)
	ctx := context.Background()

	req, err := http.NewRequest(
		"POST",
		fmt.Sprintf(
			"/tools?binaryVersion=%s",
			version.MustParseBinary("2.9.100-ubuntu-amd64"),
		), nil,
	)
	c.Assert(err, jc.ErrorIsNil)
	req.Header.Set("Content-Type", "application/x-tar-gz")
	req = req.WithContext(ctx)

	resp := &http.Response{
		Request:    req,
		StatusCode: http.StatusCreated,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(`{"tools": [{"version": "2.9.100-ubuntu-amd64"}]}`)),
	}
	resp.Header.Set("Content-Type", "application/json")

	apiCaller.EXPECT().BestFacadeVersion("ModelUpgrader").Return(1)
	apiCaller.EXPECT().HTTPClient().Return(&httprequest.Client{Doer: doer}, nil)
	doer.EXPECT().Do(req).Return(resp, nil)

	client := modelupgrader.NewClient(apiCaller)

	result, err := client.UploadTools(
		context.Background(),
		nil, version.MustParseBinary("2.9.100-ubuntu-amd64"),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, coretools.List{
		{Version: version.MustParseBinary("2.9.100-ubuntu-amd64")},
	})
}
