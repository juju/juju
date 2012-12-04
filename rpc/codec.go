package rpc
import (
	"encoding/json"
	"fmt"
	"encoding/xml"
	"io"
	"net/http"
)

type generalServerCodec struct {
	enc encoder
	dec decoder
}

type encoder interface {
	Encode(e interface{}) error
}

type decoder interface {
	Decode(e interface{}) error
}

func (c *generalServerCodec) ReadRequestHeader(req *Request) error {
	return c.dec.Decode(req)
}

func (c *generalServerCodec) ReadRequestBody(argp interface{}) error {
	return c.dec.Decode(argp)
}


func (c *generalServerCodec) WriteResponse(resp *Response, v interface{}) error {
	type generalResponse struct {
		Response *Response
		Value interface{}
	}
	return c.enc.Encode(&generalResponse{
		Response: resp,
		Value: v,
	})
}

func NewJSONServerCodec(c io.ReadWriter) ServerCodec {
	return &generalServerCodec{
		enc: json.NewEncoder(c),
		dec: json.NewDecoder(c),
	}
}

func NewXMLServerCodec(c io.ReadWriter) ServerCodec {
	return &generalServerCodec{
		enc: xml.NewEncoder(c),
		dec: xml.NewDecoder(c),
	}
}

type httpServerCodec struct {
	done bool
	w http.ResponseWriter
	req *http.Request
}

// NewHTTPCodec returns a codec which allows
// the use of the single HTTP request given in its arguments
// as a ServerCodec. URL parameters hold the arguments
// to the RPC (each individually encoded as a JSON value).
// The response is written to w in JSON format.
func NewHTTPServerCodec(w http.ResponseWriter, req *http.Request) ServerCodec {
	return &httpServerCodec{
		w: w,
		req: req,
	}
}

func (c *httpServerCodec) ReadRequestHeader(req *Request) error {
	if c.done {
		return io.EOF
	}
	c.done = true
	req.Path = c.req.URL.Path
	req.Seq = 0
	return nil
}

func (c *httpServerCodec) ReadRequestBody(argp interface{}) error {
	if err := c.req.ParseForm(); err != nil {
		return err
	}
	// Quick hack: marshal all the parameters into
	// JSON, then Unmarshal into argp.
	m := make(map[string]json.RawMessage)
	for k, vs := range c.req.Form {
		m[k] = json.RawMessage(vs[0])
	}
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, argp)
}

func (c *httpServerCodec) WriteResponse(resp *Response, v interface{}) error {
	type jsonError struct {
		Error string
		ErrorPath string
	}
	var data []byte
	c.w.Header().Set("Content-Type", "application/json")
	if resp.Error != "" {
		c.w.WriteHeader(http.StatusBadRequest)
		v = &jsonError{
			Error: resp.Error,
			ErrorPath: resp.ErrorPath,
		}
	}
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.w.Header().Set("Content-Length", fmt.Sprint(len(data)))
	_, err = c.w.Write(data)
	return err
}
