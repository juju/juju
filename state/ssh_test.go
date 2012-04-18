package state

import (
	"bufio"
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/testing"
	"local/runtime/debug"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
	"time"
)

type sshSuite struct {
	testing.LoggingSuite
}

var _ = Suite(&sshSuite{})

// fakeSSHRun represents the behaviour of the ssh command when run once.
type fakeSSHRun struct {
	Output string // The fake ssh will print this...
	Status int    // and exit with this exit code.
}

// runSpec holds the specification for the executable shell script.  It
// will use Dir as a temporary directory.  Each time it is run, a
// successive element of Runs will be used to generate the behaviour.
type runSpec struct {
	Dir  string
	Runs []fakeSSHRun
}

const backQuote = "`"

var scriptTemplate = template.Must(new(template.Template).
	Funcs(template.FuncMap{"xs": xs}).Parse(`#!/bin/sh

# We use the runcount file as a unary counter
# to determine which behaviour step we will choose.
# This means that we can generate a different,
# but predetermined output on each run.
echo -n x >> {{.Dir}}/runcount

# Decide which action to take based on the current contents of the
# runcount file.
case ` + backQuote + `cat {{.Dir}}/runcount` + backQuote + `
in{{range $i, $out := .Runs}}
{{xs $i}})
	{{if .Output}}
	echo '{{.Output}}' >&2
	{{end}}

	exit {{.Status}}
	;;
{{end}}
*)
	echo ssh run too many times >&2
	exit 5
	;;
esac
`))

// xs returns a repeated string of the letter x,
// used as a unary run counter in the generated
// shell script.
func xs(i int) string {
	return strings.Repeat("x", i+1)
}

// Rather than work out ways to get ssh to fail in all the possible
// ways, we test the internal logic of the ssh forwarder by creating an
// executable named ssh that exhibits a particular desired misbehaviour.
// errorTests holds the various misbehaviours that we wish to test.
var errorTests = []struct {
	runs []fakeSSHRun // sequence of SSH behaviours.
	err1 string       // error returned from newSSHForwarder.
	err2 string       // error returned from fwd.stop.
}{{
	[]fakeSSHRun{{"Warning: Permanently added something", 0}},
	"",
	"ssh exited silently",
}, {
	[]fakeSSHRun{{"ssh: Could not resolve hostname", 1}},
	"",
	"ssh: Could not resolve hostname",
}, {
	[]fakeSSHRun{{"ssh: cannot open key: Permission denied", 1}},
	"",
	"Invalid SSH key: .*",
}, {
	[]fakeSSHRun{
		{"cannot connect for some reason", 1},
		{"", 0},
	},
	"",
	"ssh exited silently",
}, {
	nil,
	"",
	"too many errors: .*",
}}

func (*sshSuite) TestSSHErrors(c *C) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)

	oldRetryInterval := sshRetryInterval
	defer func() {
		sshRetryInterval = oldRetryInterval
	}()
	sshRetryInterval = 0

	// Try first with no executable.
	os.Setenv("PATH", "")
	fwd, err := newSSHForwarder("somewhere.com:9999")
	c.Assert(fwd, IsNil)
	c.Assert(err, NotNil)

	// Then set the path to a temporary directory
	// and create an executable named ssh in it
	// for each test.
	dir := c.MkDir()
	os.Setenv("PATH", dir+":"+oldPath)
	rcf, err := os.Create(dir + "/runcount")
	c.Assert(err, IsNil)
	defer rcf.Close()

	var buf [10]byte
	for i, t := range errorTests {
		c.Logf("test %d", i)
		writeScript(c, dir, t.runs)
		err = rcf.Truncate(0)
		c.Assert(err, IsNil)

		fwd, err := newSSHForwarder("somewhere.com:9999")
		if t.err1 != "" {
			c.Assert(err, ErrorMatches, t.err1)
		} else {
			c.Assert(err, IsNil)
		}
		select {
		case <-fwd.Dying():
		case <-time.After(time.Second):
			c.Fatalf("timeout waiting for ssh forwarder to complete")
		}

		err = fwd.stop()
		if t.err2 != "" {
			c.Assert(err, ErrorMatches, t.err2)
		} else {
			c.Assert(err, IsNil)
		}

		if len(t.runs) > 0 {
			rcf.Seek(0, 0)
			n, err := rcf.Read(buf[:])
			c.Assert(err, IsNil)
			c.Assert(n, Equals, len(t.runs), Commentf("unexpected run count"))
		}
	}
}

func writeScript(c *C, dir string, runs []fakeSSHRun) {
	f, err := os.OpenFile(dir+"/ssh", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0777)
	c.Assert(err, IsNil)
	defer f.Close()
	err = scriptTemplate.Execute(f, runSpec{
		Dir:  dir,
		Runs: runs,
	})
	c.Assert(err, IsNil)
}

func (*sshSuite) TestSSHConnect(c *C) {
	t := newSSHTest(c)

	sshdPort := testing.FindTCPPort()
	serverPort := testing.FindTCPPort()
	c.Assert(serverPort, Not(Equals), sshdPort)

	c.Logf("sshd port %d; server port %d", sshdPort, serverPort)

	t.setSSHParams(sshdPort)
	defer t.resetSSHParams()

	c.Logf("--------- starting forwarder")
	fwd, err := newSSHForwarder(fmt.Sprintf("localhost:%d", serverPort))
	c.Assert(err, IsNil)
	c.Assert(fwd, NotNil)
	defer func() {
		err := fwd.stop()
		c.Assert(err, IsNil)
	}()
	go func() {
		err := fwd.Wait()
		c.Logf("ssh forwarder died: %v", err)
	}()

	// The SSH forwarder will have tried to start the SSH
	// client, but it will fail because there's no daemon to
	// connect to. Wait a while to allow this to happen.
	time.Sleep(500 * time.Millisecond)

	// Start the daemon and the client.
	c.Logf("--------- starting sshd")
	p := t.sshDaemon(sshdPort, serverPort)
	defer p.Kill()

	c.Logf("------------ starting client")
	clientDone := make(chan struct{})
	go t.client(fwd.localAddr, clientDone)

	// The SSH client process should now successfully start,
	// but the client will fail to connect because the server
	// has not been started. Wait a while for this to happen.
	time.Sleep(2000 * time.Millisecond)

	// Start the server to finally allow the full connection
	// to take place.
	c.Logf("--------- starting server")
	go t.server(fmt.Sprintf("localhost:%d", serverPort))

	// If the client completes, then all the intermediate units
	// have completed too, so we don't need to wait for them too.
	select {
	case <-clientDone:
	case <-time.After(5 * time.Second):
		c.Fatalf("timeout waiting for client to complete")
	}

	// TODO check log file for the following:
	//	error starting ssh (sshd not up)
	//	error connecting to remote side
	//	attempting to connect again
}
//
//func (*sshSuite) TestSSHDial(c *C) {
//	sshdPort := testing.FindTCPPort()
//	
//	srv := testing.StartZkServer()
//
//	t.setSSHParams(sshdPort)
//	defer t.resetSSHParams()
//}

// TestSSHSimpleConnect tests a slightly simpler configuration
// than TestSSHConnect
func (*sshSuite) TestSSHSimpleConnect(c *C) {
	t := newSSHTest(c)

	sshdPort := testing.FindTCPPort()
	serverPort := testing.FindTCPPort()
	c.Assert(serverPort, Not(Equals), sshdPort)
	c.Logf("sshd port %d; server port %d", sshdPort, serverPort)

	t.setSSHParams(sshdPort)
	defer t.resetSSHParams()

	c.Logf("--------- starting sshd")

	p := t.sshDaemon(sshdPort, serverPort)
	defer p.Kill()

	c.Logf("--------- starting forwarder")

	fwd, err := newSSHForwarder(fmt.Sprintf("localhost:%d", serverPort))
	c.Assert(err, IsNil)
	c.Assert(fwd, NotNil)
	defer func() {
		err := fwd.stop()
		c.Assert(err, IsNil)
	}()

	go func() {
		err := fwd.Wait()
		c.Logf("ssh forwarder died: %v", err)
	}()
	c.Logf("--------- starting server")

	go t.server(fmt.Sprintf("localhost:%d", serverPort))

	c.Logf("------------ starting client")
	clientDone := make(chan struct{})
	go t.client(fwd.localAddr, clientDone)

	// If the client completes, then all the intermediate units
	// have completed too, so we don't need to wait for them too.
	select {
	case <-clientDone:
	case <-time.After(5 * time.Second):
		c.Fatalf("timeout waiting for client to complete")
	}
}

//sshTest represents a running SSH test.
type sshTest struct {
	c   *C
	dir string		// the current directory.

	oldSSHRemotePort int
	oldSSHKeyFile string
	oldSSHUser string
}

func newSSHTest(c *C) *sshTest {
	t := &sshTest{
		c: c,
	}
	var err error
	t.dir, err = os.Getwd()
	c.Assert(err, IsNil)
	return t
}

func (t *sshTest) setSSHParams(sshdPort int) {
	t.oldSSHRemotePort = sshRemotePort
	t.oldSSHKeyFile = sshKeyFile
	t.oldSSHUser = sshUser

	sshRemotePort = sshdPort
	sshKeyFile = t.file("id_rsa")
	sshUser = ""
}

func (t *sshTest) resetSSHParams() {
	sshRemotePort = t.oldSSHRemotePort
	sshKeyFile = t.oldSSHKeyFile
	sshUser = t.oldSSHUser
}

// file returns the full path name of an ssh test file.
func (t *sshTest) file(name string) string {
	return filepath.Join(t.dir, "sshtest", name)
}

// client tests that a client can contact a server through the
// port forwarding ssh client and daemon.
func (t *sshTest) client(addr string, done chan<- struct{}) {
	defer close(done)

	t.dial(addr, "client to server 1\n")
	t.dial(addr, "client to server 2\n")
}

// dial makes repeated attempts to dial the server through the
// port forwarder.
func (t *sshTest) dial(addr string, msg string) {
	for attempt := 0; ; attempt++ {
		t.assert(attempt < 20, Equals, true)
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.c.Logf("client dial %s: %v", addr, err)
			time.Sleep(300 * time.Millisecond)
			continue
		}
		fmt.Fprint(conn, msg)
		// If the server is not yet up but the port forwarder
		// is, the connect will succeed but we will get an
		// immediate EOF when reading the response.
		r := bufio.NewReader(conn)
		line, err := r.ReadString('\n')
		conn.Close()
		if err != nil {
			t.c.Logf("client dial read error: %v", err)
			time.Sleep(300 * time.Millisecond)
			continue
		}
		t.assert(line, Equals, "reply: "+msg)
		return
	}
	panic("not reached")
}

// server is the server side of the client-server equation.
func (t *sshTest) server(addr string) {
	l, err := net.Listen("tcp", addr)
	t.assert(err, IsNil)

	t.accept1(l)
	t.accept1(l)
}

// accept1 accepts one connection, checks that
// the expected message is received and replies to it.
func (t *sshTest) accept1(l net.Listener) {
	conn, err := l.Accept()
	t.assert(err, IsNil)
	defer conn.Close()

	r := bufio.NewReader(conn)
	line, err := r.ReadString('\n')
	t.assert(err, IsNil)
	t.assert(line, Matches, "client to server [0-9]\n")
	fmt.Fprint(conn, "reply: "+line)
}

func (t *sshTest) sshDaemon(sshdPort, serverPort int) *os.Process {
	cmd := exec.Command("sshd",
		"-f", t.file("sshd_config"),
		"-h", t.file("id_rsa"),
		"-D",
		"-o", fmt.Sprintf("AuthorizedKeysFile %s", t.file("authorized_keys")),
		"-o", fmt.Sprintf("PermitOpen localhost:%d", serverPort),
		"-o", fmt.Sprintf("ListenAddress localhost:%d", sshdPort),
	)
	cmd.Env = []string{
		"HOME=" + t.file(""),
		"PATH=" + os.Getenv("PATH"),
	}
	r, err := cmd.StderrPipe()
	t.c.Assert(err, IsNil)
	cmd.Stdout = cmd.Stderr

	// Ensure that sshd is invoked with an absolute path so it
	// can re-exec itself correctly.
	cmd.Args[0] = cmd.Path
	t.c.Logf("starting sshd: %q", cmd.Args)
	err = cmd.Start()
	t.c.Assert(err, IsNil)

	go func() {
		defer r.Close()
		br := bufio.NewReader(r)
		for {
			line, _, err := br.ReadLine()
			if err != nil {
				break
			}
			t.c.Logf("sshd: %s", line)
		}
		err := cmd.Wait()
		t.c.Logf("ssh has exited: %v", err)
	}()

	return cmd.Process
}

// assert is like C.Assert except that it calls Check and then runtime.Goexit
// if the assertion fails, allowing independent goroutines to use it.
func (t *sshTest) assert(obtained interface{}, checker Checker, args ...interface{}) {
	if !t.c.Check(obtained, checker, args...) {
		t.c.Logf("callers: %s", debug.Callers(1, 10))
		runtime.Goexit()
	}
}
