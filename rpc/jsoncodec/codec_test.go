// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jsoncodec_test

import (
	"encoding/json"
	"errors"
	"io"
	"reflect"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/jsoncodec"
)

type suite struct {
	testhelpers.LoggingSuite
}

var _ = tc.Suite(&suite{})

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}

type value struct {
	X string
}

func (*suite) TestRead(c *tc.C) {
	for i, test := range []struct {
		msg        string
		expectHdr  rpc.Header
		expectBody interface{}
		expectErr  string
	}{{
		msg: `{"RequestId": 1, "Type": "foo", "Id": "id", "Request": "frob", "Params": {"X": "param"}}`,
		expectHdr: rpc.Header{
			RequestId: 1,
			Request: rpc.Request{
				Type:   "foo",
				Id:     "id",
				Action: "frob",
			},
		},
		expectErr: `reading message: version 0 not supported`,
	}, {
		msg: `{"RequestId": 2, "Error": "an error", "ErrorCode": "a code"}`,
		expectHdr: rpc.Header{
			RequestId: 2,
			Error:     "an error",
			ErrorCode: "a code",
		},
		expectErr: `reading message: version 0 not supported`,
	}, {
		msg: `{"RequestId": 3, "Response": {"X": "result"}}`,
		expectHdr: rpc.Header{
			RequestId: 3,
		},
		expectErr: `reading message: version 0 not supported`,
	}, {
		msg: `{"RequestId": 4, "Type": "foo", "Version": 2, "Id": "id", "Request": "frob", "Params": {"X": "param"}}`,
		expectHdr: rpc.Header{
			RequestId: 4,
			Request: rpc.Request{
				Type:    "foo",
				Version: 2,
				Id:      "id",
				Action:  "frob",
			},
		},
		expectErr: `reading message: version 0 not supported`,
	}, {
		msg: `{"request-id": 1, "type": "foo", "id": "id", "request": "frob", "params": {"X": "param"}}`,
		expectHdr: rpc.Header{
			RequestId: 1,
			Request: rpc.Request{
				Type:   "foo",
				Id:     "id",
				Action: "frob",
			},
			Version: 1,
		},
		expectBody: &value{X: "param"},
	}, {
		msg: `{"request-id": 2, "error": "an error", "error-code": "a code"}`,
		expectHdr: rpc.Header{
			RequestId: 2,
			Error:     "an error",
			ErrorCode: "a code",
			Version:   1,
		},
		expectBody: new(map[string]interface{}),
	}, {
		msg: `{"request-id": 2, "error": "an error", "error-code": "a code", "error-info": {"foo": "bar", "baz": true}}`,
		expectHdr: rpc.Header{
			RequestId: 2,
			Error:     "an error",
			ErrorCode: "a code",
			ErrorInfo: map[string]interface{}{
				"foo": "bar",
				"baz": true,
			},
			Version: 1,
		},
		expectBody: new(map[string]interface{}),
	}, {
		msg: `{"request-id": 3, "response": {"X": "result"}}`,
		expectHdr: rpc.Header{
			RequestId: 3,
			Version:   1,
		},
		expectBody: &value{X: "result"},
	}, {
		msg: `{"request-id": 4, "type": "foo", "version": 2, "id": "id", "request": "frob", "params": {"X": "param"}}`,
		expectHdr: rpc.Header{
			RequestId: 4,
			Request: rpc.Request{
				Type:    "foo",
				Version: 2,
				Id:      "id",
				Action:  "frob",
			},
			Version: 1,
		},
		expectBody: &value{X: "param"},
	}} {
		c.Logf("test %d", i)
		codec := jsoncodec.New(&testConn{
			readMsgs: []string{test.msg},
		})
		var hdr rpc.Header
		err := codec.ReadHeader(&hdr)
		if test.expectErr != "" {
			c.Assert(err, tc.ErrorMatches, test.expectErr)
			continue
		}
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(hdr, tc.DeepEquals, test.expectHdr)

		c.Assert(hdr.IsRequest(), tc.Equals, test.expectHdr.IsRequest())

		body := reflect.New(reflect.ValueOf(test.expectBody).Type().Elem()).Interface()
		err = codec.ReadBody(body, test.expectHdr.IsRequest())
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(body, tc.DeepEquals, test.expectBody)

		err = codec.ReadHeader(&hdr)
		c.Assert(err, tc.Equals, io.EOF)
	}
}

func (*suite) TestErrorAfterClose(c *tc.C) {
	conn := &testConn{
		err: errors.New("some error"),
	}
	codec := jsoncodec.New(conn)
	var hdr rpc.Header
	err := codec.ReadHeader(&hdr)
	c.Assert(err, tc.ErrorMatches, "receiving message: some error")

	err = codec.Close()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(conn.closed, tc.IsTrue)

	err = codec.ReadHeader(&hdr)
	c.Assert(err, tc.Equals, io.EOF)
}

func (*suite) TestWrite(c *tc.C) {
	for i, test := range []struct {
		hdr       *rpc.Header
		body      interface{}
		isRequest bool
		expect    string
		expectErr string
	}{{
		hdr: &rpc.Header{
			RequestId: 1,
			Request: rpc.Request{
				Type:   "foo",
				Id:     "id",
				Action: "frob",
			},
		},
		body:      &value{X: "param"},
		expectErr: `writing message: version 0 not supported`,
	}, {
		hdr: &rpc.Header{
			RequestId: 2,
			Error:     "an error",
			ErrorCode: "a code",
		},
		expectErr: `writing message: version 0 not supported`,
	}, {
		hdr: &rpc.Header{
			RequestId: 2,
			Error:     "an error",
			ErrorCode: "a code",
			ErrorInfo: map[string]interface{}{
				"ignored": "for version0",
			},
		},
		expectErr: `writing message: version 0 not supported`,
	}, {
		hdr: &rpc.Header{
			RequestId: 3,
		},
		body:      &value{X: "result"},
		expectErr: `writing message: version 0 not supported`,
	}, {
		hdr: &rpc.Header{
			RequestId: 4,
			Request: rpc.Request{
				Type:    "foo",
				Version: 2,
				Id:      "",
				Action:  "frob",
			},
		},
		body:      &value{X: "param"},
		expectErr: `writing message: version 0 not supported`,
	}, {
		hdr: &rpc.Header{
			RequestId: 1,
			Request: rpc.Request{
				Type:   "foo",
				Id:     "id",
				Action: "frob",
			},
			Version: 1,
		},
		body:   &value{X: "param"},
		expect: `{"request-id": 1, "type": "foo","id":"id", "request": "frob", "params": {"X": "param"}}`,
	}, {
		hdr: &rpc.Header{
			RequestId: 2,
			Error:     "an error",
			ErrorCode: "a code",
			Version:   1,
		},
		expect: `{"request-id": 2, "error": "an error", "error-code": "a code"}`,
	}, {
		hdr: &rpc.Header{
			RequestId: 2,
			Error:     "an error",
			ErrorCode: "a code",
			ErrorInfo: map[string]interface{}{
				"foo": "bar",
				"baz": true,
			},
			Version: 1,
		},
		expect: `{"request-id": 2, "error": "an error", "error-code": "a code", "error-info": {"foo": "bar", "baz": true}}`,
	}, {
		hdr: &rpc.Header{
			RequestId: 3,
			Version:   1,
		},
		body:   &value{X: "result"},
		expect: `{"request-id": 3, "response": {"X": "result"}}`,
	}, {
		hdr: &rpc.Header{
			RequestId: 4,
			Request: rpc.Request{
				Type:    "foo",
				Version: 2,
				Id:      "",
				Action:  "frob",
			},
			Version: 1,
		},
		body:   &value{X: "param"},
		expect: `{"request-id": 4, "type": "foo", "version": 2, "request": "frob", "params": {"X": "param"}}`,
	}} {
		c.Logf("test %d", i)
		var conn testConn
		codec := jsoncodec.New(&conn)
		err := codec.WriteMessage(test.hdr, test.body)
		if test.expectErr != "" {
			c.Assert(err, tc.ErrorMatches, test.expectErr)
			continue
		}
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(conn.writeMsgs, tc.HasLen, 1)

		assertJSONEqual(c, conn.writeMsgs[0], test.expect)
	}
}

func (*suite) TestDumpRequest(c *tc.C) {
	for i, test := range []struct {
		hdr    rpc.Header
		body   interface{}
		expect string
	}{{
		hdr: rpc.Header{
			RequestId: 1,
			Request: rpc.Request{
				Type:   "Foo",
				Id:     "id",
				Action: "Something",
			},
		},
		body:   struct{ Arg string }{Arg: "an arg"},
		expect: `"version 0 not supported"`,
	}, {
		hdr: rpc.Header{
			RequestId: 2,
		},
		body:   struct{ Ret string }{Ret: "return value"},
		expect: `"version 0 not supported"`,
	}, {
		hdr: rpc.Header{
			RequestId: 3,
		},
		expect: `"version 0 not supported"`,
	}, {
		hdr: rpc.Header{
			RequestId: 4,
			Error:     "an error",
			ErrorCode: "an error code",
		},
		expect: `"version 0 not supported"`,
	}, {
		hdr: rpc.Header{
			RequestId: 5,
		},
		body:   make(chan int),
		expect: `"version 0 not supported"`,
	}, {
		hdr: rpc.Header{
			RequestId: 1,
			Request: rpc.Request{
				Type:    "Foo",
				Version: 2,
				Id:      "id",
				Action:  "Something",
			},
		},
		body:   struct{ Arg string }{Arg: "an arg"},
		expect: `"version 0 not supported"`,
	}, {
		hdr: rpc.Header{
			RequestId: 1,
			Request: rpc.Request{
				Type:   "Foo",
				Id:     "id",
				Action: "Something",
			},
			Version: 1,
		},
		body:   struct{ Arg string }{Arg: "an arg"},
		expect: `{"request-id":1,"type":"Foo","id":"id","request":"Something","params":{"Arg":"an arg"}}`,
	}, {
		hdr: rpc.Header{
			RequestId: 2,
			Version:   1,
		},
		body:   struct{ Ret string }{Ret: "return value"},
		expect: `{"request-id":2,"response":{"Ret":"return value"}}`,
	}, {
		hdr: rpc.Header{
			RequestId: 3,
			Version:   1,
		},
		expect: `{"request-id":3}`,
	}, {
		hdr: rpc.Header{
			RequestId: 4,
			Error:     "an error",
			ErrorCode: "an error code",
			Version:   1,
		},
		expect: `{"request-id":4,"error":"an error","error-code":"an error code"}`,
	}, {
		hdr: rpc.Header{
			RequestId: 5,
			Version:   1,
		},
		body:   make(chan int),
		expect: `"marshal error: json: unsupported type: chan int"`,
	}, {
		hdr: rpc.Header{
			RequestId: 1,
			Request: rpc.Request{
				Type:    "Foo",
				Version: 2,
				Id:      "id",
				Action:  "Something",
			},
			Version: 1,
		},
		body:   struct{ Arg string }{Arg: "an arg"},
		expect: `{"request-id":1,"type":"Foo","version":2,"id":"id","request":"Something","params":{"Arg":"an arg"}}`,
	}} {
		c.Logf("test %d; %#v", i, test.hdr)
		data := jsoncodec.DumpRequest(&test.hdr, test.body)
		c.Check(string(data), tc.Equals, test.expect)
	}
}

// assertJSONEqual compares the json strings v0
// and v1 ignoring white space.
func assertJSONEqual(c *tc.C, v0, v1 string) {
	var m0, m1 interface{}
	err := json.Unmarshal([]byte(v0), &m0)
	c.Assert(err, tc.ErrorIsNil)
	err = json.Unmarshal([]byte(v1), &m1)
	c.Assert(err, tc.ErrorIsNil)
	data0, err := json.Marshal(m0)
	c.Assert(err, tc.ErrorIsNil)
	data1, err := json.Marshal(m1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(data0), tc.Equals, string(data1))
}

type testConn struct {
	readMsgs  []string
	err       error
	writeMsgs []string
	closed    bool
}

func (c *testConn) Receive(msg interface{}) error {
	if len(c.readMsgs) > 0 {
		s := c.readMsgs[0]
		c.readMsgs = c.readMsgs[1:]
		return json.Unmarshal([]byte(s), msg)
	}
	if c.err != nil {
		return c.err
	}
	return io.EOF
}

func (c *testConn) Send(msg interface{}) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	c.writeMsgs = append(c.writeMsgs, string(data))
	return nil
}

func (c *testConn) Close() error {
	c.closed = true
	return nil
}
