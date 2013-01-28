package rpc

type ClientCodec interface {
	WriteRequest(*Request, interface{}) error
	ReadResponseHeader(*Response) error
	ReadResponseBody(interface{}) error
}

type Client struct {
	codec ClientCodec
	reqId   uint64
}

func NewClientWithCodec(codec ClientCodec) *Client {
	return &Client{
		codec: codec,
	}
}

// RemoteError represents an error returned from an RPC server.
type RemoteError struct {
	Message string
}

func (e *RemoteError) Error() string {
	return e.Message
}

// Call invokes the named action on the object of the given
// type with the given id. If the action fails remotely, the
// returned error will be a RemoteError.
func (c *Client) Call(objType, id, action string, args, reply interface{}) error {
	// TODO concurrent calls
	c.reqId++
	req := &Request{
		RequestId:  c.reqId,
		Type: objType,
		Id: id,
		Action: action,
	}
	if err := c.codec.WriteRequest(req, args); err != nil {
		return err
	}
	var resp Response
	if err := c.codec.ReadResponseHeader(&resp); err != nil {
		return err
	}
	if resp.Error != "" {
		reply = nil
	}
	if err := c.codec.ReadResponseBody(reply); err != nil && resp.Error == "" {
		return err
	}
	if resp.Error != "" {
		return &RemoteError{resp.Error}
	}
	return nil
}
