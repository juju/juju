package apiserver

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
)

var websocketUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func websocketServer(w http.ResponseWriter, req *http.Request, handler func(ws *websocket.Conn)) {
	conn, err := websocketUpgrader.Upgrade(w, req, nil)
	if err != nil {
		logger.Errorf("problem initiating websocket: %v", err)
		return
	}
	handler(conn)
}

// sendInitialErrorV0 writes out the error as a params.ErrorResult serialized
// with JSON with a new line character at the end.
//
// This is a hangover from the initial debug-log streaming endoing where the
// client read the first line, and then just got a stream of data. We should
// look to version the streaming endpoints to get rid of the trailing newline
// character for message based connections, which is all of them now.
func sendInitialErrorV0(ws *websocket.Conn, err error) error {
	wrapped := &params.ErrorResult{
		Error: common.ServerError(err),
	}

	body, err := json.Marshal(wrapped)
	if err != nil {
		errors.Annotatef(err, "cannot marshal error %#v", wrapped)
		return err
	}
	body = append(body, '\n')

	writer, err := ws.NextWriter(websocket.TextMessage)
	if err != nil {
		return errors.Annotate(err, "problem getting writer")
	}
	defer writer.Close()
	_, err = writer.Write(body)

	if wrapped.Error != nil {
		// Tell the other end we are closing.
		ws.WriteMessage(websocket.CloseMessage, []byte{})
	}

	return errors.Trace(err)
}
