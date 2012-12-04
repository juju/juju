package rpc
import (
	"encoding/json"
	"encoding/xml"
	"io"
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

//
//// NewHTTPCodec returns a codec which allows
//// the use of the single HTTP request given in its arguments
//// as a ServerCodec. URL parameters hold the arguments
//// to the RPC; the response is written to w in JSON format.
//func NewHTTPServerCodec(w http.ResponseWriter, r *http.Request) {
//	
//}
