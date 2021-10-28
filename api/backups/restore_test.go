// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"github.com/golang/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/backups"
	"github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/core/params"
)

type restoreSuite struct {
}

var _ = gc.Suite(&restoreSuite{})

func (s *restoreSuite) TestRestoreFromFileBackupExistsOnController(c *gc.C) {
	mockController := gomock.NewController(c)
	mockBackupFacadeCaller := mocks.NewMockFacadeCaller(mockController)
	mockBackupClientFacade := mocks.NewMockClientFacade(mockController)
	mockBackupClientFacade.EXPECT().Close().AnyTimes()

	// testBackupResults represents the parameters passed in by the client.
	testBackupResults := params.BackupsMetadataResult{
		Checksum: "testCheckSum",
	}

	// This listBackupResults represents what is passed back from server.
	testBackupsListResults := params.BackupsListResult{
		List: []params.BackupsMetadataResult{testBackupResults},
	}

	// resultBackupList is an out param
	resultBackupList := &params.BackupsListResult{}
	args := params.BackupsListArgs{}

	// Upload should never be called. If it is the test will fail.
	gomock.InOrder(
		mockBackupFacadeCaller.EXPECT().FacadeCall("PrepareRestore", nil, gomock.Any()),
		mockBackupFacadeCaller.EXPECT().FacadeCall("List", args, resultBackupList).SetArg(2, testBackupsListResults),
		mockBackupFacadeCaller.EXPECT().FacadeCall("Restore", gomock.Any(), gomock.Any()),
		mockBackupFacadeCaller.EXPECT().FacadeCall("FinishRestore", gomock.Any(), gomock.Any()),
	)

	connFunc := func() (*backups.Client, error) {
		return backups.MakeClient(mockBackupClientFacade, mockBackupFacadeCaller, nil), nil
	}
	mockBackupsClient, _ := connFunc()
	mockBackupsClient.RestoreReader(nil, &testBackupResults, connFunc)
}
