package api

import (
	"code.google.com/p/go.net/websocket"
	"errors"
	"fmt"
)

type State struct {
	conn *websocket.Conn
}

type Info struct {
	Addr string
}

func Open(info *Info) (*State, error) {
	conn, err := websocket.Dial("ws://"+info.Addr+"/", "", "http://localhost/")
	if err != nil {
		return nil, err
	}
	return &State{
		conn: conn,
	}, nil
}

func (s *State) Close() error {
	return s.conn.Close()
}

// Request is a placeholder for an arbitrary operation in the state API.
// Currently it simply returns the instance id of the machine with the
// id given by the request.
func (s *State) Request(req string) (string, error) {
	err := websocket.JSON.Send(s.conn, rpcRequest{req})
	if err != nil {
		return "", fmt.Errorf("cannot send request: %v", err)
	}
	var resp rpcResponse
	err = websocket.JSON.Receive(s.conn, &resp)
	if err != nil {
		return "", fmt.Errorf("cannot receive response: %v", err)
	}
	if resp.Error != "" {
		return "", errors.New(resp.Error)
	}
	return resp.Response, nil
}
