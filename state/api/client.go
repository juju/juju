package api

import (
	"code.google.com/p/go.net/websocket"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"launchpad.net/juju-core/cert"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/rpc"
	"launchpad.net/juju-core/trivial"
	"time"
)

type State struct {
	client *rpc.Client
	conn   *websocket.Conn
}

// Info encapsulates information about a server holding juju state and
// can be used to make a connection to it.
type Info struct {
	// Addrs holds the addresses of the state servers.
	Addrs []string

	// CACert holds the CA certificate that will be used
	// to validate the state server's certificate, in PEM format.
	CACert []byte

	// EntityName holds the name of the entity that is connecting.
	// If this and the password are empty, no login attempt will be made
	// (this is to allow tests to access the API to check that operations
	// fail when not logged in).
	EntityName string

	// Password holds the password for the administrator or connecting entity.
	Password string
}

var openAttempt = trivial.AttemptStrategy{
	Total: 5 * time.Minute,
	Delay: 500 * time.Millisecond,
}

func Open(info *Info) (*State, error) {
	// TODO Select a random address from info.Addrs
	// and only fail when we've tried all the addresses.
	// TODO what does "origin" really mean, and is localhost always ok?
	cfg, err := websocket.NewConfig("wss://"+info.Addrs[0]+"/", "http://localhost/")
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	xcert, err := cert.ParseCert(info.CACert)
	if err != nil {
		return nil, err
	}
	pool.AddCert(xcert)
	cfg.TlsConfig = &tls.Config{
		RootCAs:    pool,
		ServerName: "anything",
	}
	var conn *websocket.Conn
	for a := openAttempt.Start(); a.Next(); {
		log.Printf("state/api: dialling %q", cfg.Location)
		conn, err = websocket.DialConfig(cfg)
		if err == nil {
			break
		}
		log.Printf("state/api: %v", err)
	}
	if err != nil {
		return nil, err
	}
	log.Printf("state/api: connection established")

	client := rpc.NewClientWithCodec(&clientCodec{conn: conn})
	st := &State{
		client: client,
		conn:   conn,
	}
	if info.EntityName != "" || info.Password != "" {
		if err := st.Login(info.EntityName, info.Password); err != nil {
			conn.Close()
			return nil, err
		}
	}
	return st, nil
}

func (s *State) call(objType, id, request string, params, response interface{}) error {
	err := s.client.Call(objType, id, request, params, response)
	return clientError(err)
}

func (s *State) Close() error {
	return s.client.Close()
}

type clientReq struct {
	RequestId uint64
	Type      string
	Id        string
	Request   string
	Params    interface{}
}

type clientResp struct {
	RequestId uint64
	Error     string
	ErrorCode string
	Response  json.RawMessage
}

type clientCodec struct {
	conn *websocket.Conn
	resp clientResp
}

func (c *clientCodec) Close() error {
	return c.conn.Close()
}

func (c *clientCodec) WriteRequest(req *rpc.Request, p interface{}) error {
	return websocket.JSON.Send(c.conn, &clientReq{
		RequestId: req.RequestId,
		Type:      req.Type,
		Id:        req.Id,
		Request:   req.Request,
		Params:    p,
	})
}

func (c *clientCodec) ReadResponseHeader(resp *rpc.Response) error {
	c.resp = clientResp{}
	if err := websocket.JSON.Receive(c.conn, &c.resp); err != nil {
		return err
	}
	resp.RequestId = c.resp.RequestId
	resp.Error = c.resp.Error
	resp.ErrorCode = c.resp.ErrorCode
	return nil
}

func (c *clientCodec) ReadResponseBody(body interface{}) error {
	if body == nil {
		return nil
	}
	return json.Unmarshal(c.resp.Response, body)
}
