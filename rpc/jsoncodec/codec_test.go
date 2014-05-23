package jsoncodec_test

import (
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"regexp"
	stdtesting "testing"

	"github.com/juju/loggo"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/rpc"
	"launchpad.net/juju-core/rpc/jsoncodec"
	"launchpad.net/juju-core/testing"
)

type suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&suite{})

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

type value struct {
	X string
}

var readTests = []struct {
	msg        string
	expectHdr  rpc.Header
	expectBody interface{}
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
	expectBody: &value{X: "param"},
}, {
	msg: `{"RequestId": 2, "Error": "an error", "ErrorCode": "a code"}`,
	expectHdr: rpc.Header{
		RequestId: 2,
		Error:     "an error",
		ErrorCode: "a code",
	},
	expectBody: new(map[string]interface{}),
}, {
	msg: `{"RequestId": 3, "Response": {"X": "result"}}`,
	expectHdr: rpc.Header{
		RequestId: 3,
	},
	expectBody: &value{X: "result"},
}}

func (*suite) TestRead(c *gc.C) {
	for i, test := range readTests {
		c.Logf("test %d", i)
		codec := jsoncodec.New(&testConn{
			readMsgs: []string{test.msg},
		})
		var hdr rpc.Header
		err := codec.ReadHeader(&hdr)
		c.Assert(err, gc.IsNil)
		c.Assert(hdr, gc.DeepEquals, test.expectHdr)

		c.Assert(hdr.IsRequest(), gc.Equals, test.expectHdr.IsRequest())

		body := reflect.New(reflect.ValueOf(test.expectBody).Type().Elem()).Interface()
		err = codec.ReadBody(body, test.expectHdr.IsRequest())
		c.Assert(err, gc.IsNil)
		c.Assert(body, gc.DeepEquals, test.expectBody)

		err = codec.ReadHeader(&hdr)
		c.Assert(err, gc.Equals, io.EOF)
	}
}

func (*suite) TestReadHeaderLogsRequests(c *gc.C) {
	codecLogger := loggo.GetLogger("juju.rpc.jsoncodec")
	defer codecLogger.SetLogLevel(codecLogger.LogLevel())
	codecLogger.SetLogLevel(loggo.TRACE)
	msg := `{"RequestId":1,"Type": "foo","Id": "id","Request":"frob","Params":{"X":"param"}}`
	codec := jsoncodec.New(&testConn{
		readMsgs: []string{msg, msg, msg},
	})
	// Check that logging is off by default
	var h rpc.Header
	err := codec.ReadHeader(&h)
	c.Assert(err, gc.IsNil)
	c.Assert(c.GetTestLog(), gc.Matches, "")

	// Check that we see a log message when we switch logging on.
	codec.SetLogging(true)
	err = codec.ReadHeader(&h)
	c.Assert(err, gc.IsNil)
	c.Assert(c.GetTestLog(), gc.Matches, ".*TRACE juju.rpc.jsoncodec <- "+regexp.QuoteMeta(msg)+`\n`)

	// Check that we can switch it off again
	codec.SetLogging(false)
	err = codec.ReadHeader(&h)
	c.Assert(err, gc.IsNil)
	c.Assert(c.GetTestLog(), gc.Matches, ".*TRACE juju.rpc.jsoncodec <- "+regexp.QuoteMeta(msg)+`\n`)
}

func (*suite) TestWriteMessageLogsRequests(c *gc.C) {
	codecLogger := loggo.GetLogger("juju.rpc.jsoncodec")
	defer codecLogger.SetLogLevel(codecLogger.LogLevel())
	codecLogger.SetLogLevel(loggo.TRACE)
	codec := jsoncodec.New(&testConn{})
	h := rpc.Header{
		RequestId: 1,
		Request: rpc.Request{
			Type:   "foo",
			Id:     "id",
			Action: "frob",
		},
	}

	// Check that logging is off by default
	err := codec.WriteMessage(&h, value{X: "param"})
	c.Assert(err, gc.IsNil)
	c.Assert(c.GetTestLog(), gc.Matches, "")

	// Check that we see a log message when we switch logging on.
	codec.SetLogging(true)
	err = codec.WriteMessage(&h, value{X: "param"})
	c.Assert(err, gc.IsNil)
	msg := `{"RequestId":1,"Type":"foo","Id":"id","Request":"frob","Params":{"X":"param"}}`
	c.Assert(c.GetTestLog(), gc.Matches, `.*TRACE juju.rpc.jsoncodec -> `+regexp.QuoteMeta(msg)+`\n`)

	// Check that we can switch it off again
	codec.SetLogging(false)
	err = codec.WriteMessage(&h, value{X: "param"})
	c.Assert(err, gc.IsNil)
	c.Assert(c.GetTestLog(), gc.Matches, `.*TRACE juju.rpc.jsoncodec -> `+regexp.QuoteMeta(msg)+`\n`)
}

func (*suite) TestConcurrentSetLoggingAndWrite(c *gc.C) {
	// If log messages are not set atomically, this
	// test will fail when run under the race detector.
	codec := jsoncodec.New(&testConn{})
	done := make(chan struct{})
	go func() {
		codec.SetLogging(true)
		done <- struct{}{}
	}()
	h := rpc.Header{
		RequestId: 1,
		Request: rpc.Request{
			Type:   "foo",
			Id:     "id",
			Action: "frob",
		},
	}
	err := codec.WriteMessage(&h, value{X: "param"})
	c.Assert(err, gc.IsNil)
	<-done
}

func (*suite) TestConcurrentSetLoggingAndRead(c *gc.C) {
	// If log messages are not set atomically, this
	// test will fail when run under the race detector.
	msg := `{"RequestId":1,"Type": "foo","Id": "id","Request":"frob","Params":{"X":"param"}}`
	codec := jsoncodec.New(&testConn{
		readMsgs: []string{msg, msg, msg},
	})
	done := make(chan struct{})
	go func() {
		codec.SetLogging(true)
		done <- struct{}{}
	}()
	var h rpc.Header
	err := codec.ReadHeader(&h)
	c.Assert(err, gc.IsNil)
	<-done
}

func (*suite) TestErrorAfterClose(c *gc.C) {
	conn := &testConn{
		err: errors.New("some error"),
	}
	codec := jsoncodec.New(conn)
	var hdr rpc.Header
	err := codec.ReadHeader(&hdr)
	c.Assert(err, gc.ErrorMatches, "error receiving message: some error")

	err = codec.Close()
	c.Assert(err, gc.IsNil)
	c.Assert(conn.closed, gc.Equals, true)

	err = codec.ReadHeader(&hdr)
	c.Assert(err, gc.Equals, io.EOF)
}

var writeTests = []struct {
	hdr       *rpc.Header
	body      interface{}
	isRequest bool
	expect    string
}{{
	hdr: &rpc.Header{
		RequestId: 1,
		Request: rpc.Request{
			Type:   "foo",
			Id:     "id",
			Action: "frob",
		},
	},
	body:   &value{X: "param"},
	expect: `{"RequestId": 1, "Type": "foo","Id":"id", "Request": "frob", "Params": {"X": "param"}}`,
}, {
	hdr: &rpc.Header{
		RequestId: 2,
		Error:     "an error",
		ErrorCode: "a code",
	},
	expect: `{"RequestId": 2, "Error": "an error", "ErrorCode": "a code"}`,
}, {
	hdr: &rpc.Header{
		RequestId: 3,
	},
	body:   &value{X: "result"},
	expect: `{"RequestId": 3, "Response": {"X": "result"}}`,
}}

func (*suite) TestWrite(c *gc.C) {
	for i, test := range writeTests {
		c.Logf("test %d", i)
		var conn testConn
		codec := jsoncodec.New(&conn)
		err := codec.WriteMessage(test.hdr, test.body)
		c.Assert(err, gc.IsNil)
		c.Assert(conn.writeMsgs, gc.HasLen, 1)

		assertJSONEqual(c, conn.writeMsgs[0], test.expect)
	}
}

var dumpRequestTests = []struct {
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
	expect: `{"RequestId":1,"Type":"Foo","Id":"id","Request":"Something","Params":{"Arg":"an arg"}}`,
}, {
	hdr: rpc.Header{
		RequestId: 2,
	},
	body:   struct{ Ret string }{Ret: "return value"},
	expect: `{"RequestId":2,"Response":{"Ret":"return value"}}`,
}, {
	hdr: rpc.Header{
		RequestId: 3,
	},
	expect: `{"RequestId":3}`,
}, {
	hdr: rpc.Header{
		RequestId: 4,
		Error:     "an error",
		ErrorCode: "an error code",
	},
	expect: `{"RequestId":4,"Error":"an error","ErrorCode":"an error code"}`,
}, {
	hdr: rpc.Header{
		RequestId: 5,
	},
	body:   make(chan int),
	expect: `"marshal error: json: unsupported type: chan int"`,
}}

func (*suite) TestDumpRequest(c *gc.C) {
	for i, test := range dumpRequestTests {
		c.Logf("test %d; %#v", i, test.hdr)
		data := jsoncodec.DumpRequest(&test.hdr, test.body)
		c.Check(string(data), gc.Equals, test.expect)
	}
}

// assertJSONEqual compares the json strings v0
// and v1 ignoring white space.
func assertJSONEqual(c *gc.C, v0, v1 string) {
	var m0, m1 interface{}
	err := json.Unmarshal([]byte(v0), &m0)
	c.Assert(err, gc.IsNil)
	err = json.Unmarshal([]byte(v1), &m1)
	c.Assert(err, gc.IsNil)
	data0, err := json.Marshal(m0)
	c.Assert(err, gc.IsNil)
	data1, err := json.Marshal(m1)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data0), gc.Equals, string(data1))
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
