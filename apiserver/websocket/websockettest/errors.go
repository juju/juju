// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package websockettest

import (
	"bufio"
	"encoding/json"

	"github.com/gorilla/websocket"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
)

// AssertWebsocketClosed checks that the given websocket connection
// is closed.
func AssertWebsocketClosed(c *gc.C, ws *websocket.Conn) {
	_, _, err := ws.NextReader()
	goodClose := []int{
		websocket.CloseNormalClosure,
		websocket.CloseGoingAway,
		websocket.CloseNoStatusReceived,
	}
	c.Logf("%#v", err)
	c.Assert(websocket.IsCloseError(err, goodClose...), jc.IsTrue)
}

// AssertJSONError checks the JSON encoded error returned by the log
// and logsink APIs matches the expected value.
func AssertJSONError(c *gc.C, ws *websocket.Conn, expected string) {
	errResult := ReadJSONErrorLine(c, ws)
	c.Assert(errResult.Error, gc.NotNil)
	c.Assert(errResult.Error.Message, gc.Matches, expected)
}

// AssertJSONInitialErrorNil checks the JSON encoded error returned by the log
// and logsink APIs are nil.
func AssertJSONInitialErrorNil(c *gc.C, ws *websocket.Conn) {
	errResult := ReadJSONErrorLine(c, ws)
	c.Assert(errResult.Error, gc.IsNil)
}

// ReadJSONErrorLine returns the error line returned by the log and
// logsink APIS.
func ReadJSONErrorLine(c *gc.C, ws *websocket.Conn) params.ErrorResult {
	messageType, reader, err := ws.NextReader()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(messageType, gc.Equals, websocket.TextMessage)
	line, err := bufio.NewReader(reader).ReadSlice('\n')
	c.Assert(err, jc.ErrorIsNil)
	var errResult params.ErrorResult
	err = json.Unmarshal(line, &errResult)
	c.Assert(err, jc.ErrorIsNil)
	return errResult
}
