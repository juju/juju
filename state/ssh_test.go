package state

import (
	. "launchpad.net/gocheck"
	"os"
	"strings"
	"text/template"
)

type sshSuite struct{}

var _ = Suite(sshSuite{})

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
	"Invalid SSH key",
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

func (sshSuite) TestSSHErrors(c *C) {
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

		<-fwd.Dying()

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

//func (sshSuite) TestSSHConnect(c *C) {
//	sshdPort, err := FindPort()
//	c.Assert(err, IsNil)
//	serverPort, err := FindPort()
//	c.Assert(err, IsNil)
//	c.Assert(serverPort, Not(Equals), sshdPort)
//
//	// defer set remoteSSHPort = current value
//	remoteSSHport = sshdPort
//	fwd, err := newSSHForwarder("localhost:"+port)
//	time.Sleep(500 * time.Millisecond)		// wait until the first ssh attempt has failed.
//	go sshClient(c, fwd.localAddress, wg)
//	go sshDaemon(c, sshdPort, wg)
//	time.Sleep(500 * time.Millisecond)		// wait until the port attempt fails	go server(c, remotePort, wg)
//	wg.Wait()
//	check log file for:
//		error starting ssh (sshd not up)
//		error connecting to remote side
//		attempting to connect again
//}
//
//func sshClient(c *C, addr string, wg *sync.WaitGroup) {
//	defer wg.Done()
//	responses := []string{"error on first connect\n", "server to client\n"}
//	for i := 0; i < 2; i++ {
//		dialClient(c, addr, responses[i])
//	}
//}
//
//type sshTest struct{
//	done chan struct{}
//	c *C
//}
//
//func (t sshTest) dial(addr string, expectedResponse string) {
//	defer t.wg.Done()
//	conn, err := net.Dial("tcp", addr)
//	t.assert(err, NotNil)
//
//	defer conn.Close()
//	fmt.Fprintf(conn, "client to server\n")
//
//	r := bufio.NewReader(conn)
//	line, err := r.ReadString('\n')
//	t.assert(err, IsNil)
//	t.assert(line, Equals, "server to client\n")
//}
//
//func (t sshTest) server() {
//	listen for dial attempt
//	read message, write error
//
//	listen for dial attempt
//	read message, write message
//
//	signal done
//}
//
//func (t sshTest) sshd(sshdPort int, wg *sync.WaitGroup) {
//	start sshd (with specific authorized keys and permissions)
//	HOME=$tmpdir sshd -D -p $sshport nie
//}	
//
//// assert is like C.Assert except that it calls Check and then runtime.Goexit
//// if the assertion fails.
//func (t sshTest) assert(c *C, obtained interface{}, checker Checker, args ...interface{}) {
//	if !c.Check(obtained, checker, args...) {
//		runtime.Goexit()
//	}
//}
//
