// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package websockettest

import (
	"bufio"
	"encoding/json"

	"github.com/gorilla/websocket"
	"github.com/juju/tc"

	"github.com/juju/juju/rpc/params"
)

// AssertWebsocketClosed checks that the given websocket connection
// is closed.
func AssertWebsocketClosed(c *tc.C, ws *websocket.Conn) {
	_, _, err := ws.NextReader()
	goodClose := []int{
		websocket.CloseNormalClosure,
		websocket.CloseGoingAway,
		websocket.CloseNoStatusReceived,
	}
	c.Logf("%#v", err)
	c.Assert(websocket.IsCloseError(err, goodClose...), tc.IsTrue)
}

// AssertJSONError checks the JSON encoded error returned by the log
// and logsink APIs matches the expected value.
func AssertJSONError(c *tc.C, ws *websocket.Conn, expected string) {
	errResult := ReadJSONErrorLine(c, ws)
	c.Assert(errResult.Error, tc.NotNil)
	c.Assert(errResult.Error.Message, tc.Matches, expected)
}

// AssertJSONInitialErrorNil checks the JSON encoded error returned by the log
// and logsink APIs are nil.
func AssertJSONInitialErrorNil(c *tc.C, ws *websocket.Conn) {
	errResult := ReadJSONErrorLine(c, ws)
	c.Assert(errResult.Error, tc.IsNil)
}

// ReadJSONErrorLine returns the error line returned by the log and
// logsink APIS.
func ReadJSONErrorLine(c *tc.C, ws *websocket.Conn) params.ErrorResult {
	messageType, reader, err := ws.NextReader()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(messageType, tc.Equals, websocket.TextMessage)
	line, err := bufio.NewReader(reader).ReadSlice('\n')
	c.Assert(err, tc.ErrorIsNil)
	var errResult params.ErrorResult
	err = json.Unmarshal(line, &errResult)
	c.Assert(err, tc.ErrorIsNil)
	return errResult
}
