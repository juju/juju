package rpc
import (
	"fmt"
)

type ClientCodec interface {
	WriteRequest(*Request, interface{}) error
	ReadResponseHeader(*Response) error
	ReadResponseBody(interface{}) error
}

type Client struct {
	codec ClientCodec
	seq uint64
}

func NewClientWithCodec(codec ClientCodec) *Client {
	return &Client{
		codec: codec,
	}
}

// RemoteError represents an error returned from
// an RPC server.
// TODO integrate with jsonError, pathError and Response?
type RemoteError struct {
	Message string
	Path string
}

func (e *RemoteError) Error() string {
	if e.Path == "" {
		return e.Message
	}
	return fmt.Sprintf("error at %q: %s", e.Message, e.Path)
}

func (c *Client) Call(path string, args, reply interface{}) error {
	// TODO concurrent calls
	c.seq++
	req := &Request{
		Path: path,
		Seq: c.seq,
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
		return &RemoteError{
			Message: resp.Error,
			Path: resp.ErrorPath,
		}
	}
	return nil
}
