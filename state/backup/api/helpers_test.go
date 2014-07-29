// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/api/params"
)

func (b *BackupSuite) newHTTPResponse(c *gc.C, statusCode int, data []byte) *http.Response {
	body := bytes.NewBuffer(data)
	resp := http.Response{
		StatusCode: statusCode,
		Body:       ioutil.NopCloser(body),
	}
	return &resp
}

func (b *BackupSuite) newDataResponse(c *gc.C, data string) *http.Response {
	return b.newHTTPResponse(c, http.StatusOK, []byte(data))
}

func (b *BackupSuite) newJSONResponse(c *gc.C, statusCode int, result interface{}) *http.Response {
	data, err := json.Marshal(result)
	c.Assert(err, gc.IsNil)
	return b.newHTTPResponse(c, statusCode, data)
}

func (b *BackupSuite) newHTTPFailure(c *gc.C, msg, code string) *http.Response {
	failure := params.BackupResponse{Error: msg}
	return b.newJSONResponse(c, http.StatusInternalServerError, &failure)
}
