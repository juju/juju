package state

import (
	"bufio"
	"fmt"
	"io"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju-core/log"
	"launchpad.net/tomb"
	"net"
	"os/exec"
	"strings"
	"time"
)

// These variables would be constants but they are varied
// for testing purposes.
var (
	sshRemotePort    = 22
	sshRetryInterval = time.Second
	sshUser          = "ubuntu@"
)

// sshDial dials the ZooKeeper instance at addr through
// an SSH proxy.
func sshDial(addr, keyFile string) (fwd *sshForwarder, con *zookeeper.Conn, err error) {
	fwd, err = newSSHForwarder(addr, keyFile)
	if err != nil {
		return nil, nil, err
	}
	defer errorContextf(&err, "cannot dial ZooKeeper via SSH at address %s", addr)
	zk, session, err := zookeeper.Dial(fwd.localAddr, zkTimeout)
	if err != nil {
		fwd.stop()
		return nil, nil, err
	}

	select {
	case e := <-session:
		if !e.Ok() {
			fwd.stop()
			return nil, nil, fmt.Errorf("critical zk event: %v", e)
		}
	case <-fwd.Dead():
		return nil, nil, fwd.stop()
	}
	return fwd, zk, nil
}

type sshForwarder struct {
	tomb.Tomb
	localAddr  string
	remoteHost string
	remotePort string
	keyFile    string
}

// newSSHForwarder starts an ssh proxy connecting to the
// remote TCP address. The localAddr field holds the
// name of the local proxy address. If keyFile is non-empty,
// it should name a file containing a private identity key.
func newSSHForwarder(remoteAddr, keyFile string) (fwd *sshForwarder, err error) {
	defer errorContextf(&err, "cannot start SSH proxy for address %s", remoteAddr)
	remoteHost, remotePort, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return nil, err
	}
	localPort, err := chooseZkPort()
	if err != nil {
		return nil, fmt.Errorf("cannot choose local port: %v", err)
	}
	fwd = &sshForwarder{
		localAddr:  fmt.Sprintf("localhost:%d", localPort),
		remoteHost: remoteHost,
		remotePort: remotePort,
		keyFile:    keyFile,
	}
	proc, err := fwd.start()
	if err != nil {
		return nil, err
	}
	go fwd.run(proc)
	return fwd, nil
}

func (fwd *sshForwarder) stop() error {
	fwd.Kill(nil)
	return fwd.Wait()
}

// run is called with the ssh process already active.
// It loops, restarting ssh until it gets an unrecoverable error.
func (fwd *sshForwarder) run(proc *sshProc) {
	defer fwd.Done()
	restartCount := 0
	startTime := time.Now()

	// waitErrTimeout and waitErr become valid when the ssh client has exited.
	var (
		waitErrTimeout <-chan time.Time
		waitErr        error
	)
	for {
		select {
		case <-fwd.Dying():
			proc.stop()
			return

		case sshErr := <-proc.error:
			log.Printf("state: ssh error (will retry: %v): %v", !sshErr.fatal, sshErr)
			if sshErr.fatal {
				proc.stop()
				fwd.Kill(sshErr)
				return
			}
			// If the ssh process dies repeatedly after running
			// for a very short time and we don't recognise the
			// error, we assume that something unknown is
			// going wrong and quit.
			if sshErr.unknown && time.Now().Sub(startTime) < 200*time.Millisecond {
				if restartCount++; restartCount > 10 {
					proc.stop()
					log.Printf("state: too many ssh errors")
					fwd.Kill(fmt.Errorf("too many errors: %v", sshErr))
					return
				}
			} else {
				restartCount = 0
			}

		case waitErr = <-proc.wait:
			// If ssh has exited, we'll wait a little while
			// in case we've got the exit status before we've
			// received the error.
			waitErrTimeout = time.After(100 * time.Millisecond)
			continue

		case <-waitErrTimeout:
			log.Printf("state: ssh client exited silently: %v", waitErr)
			// We only get here if ssh exits when no fatal
			// errors have been printed by ssh.  In that
			// case, it's probably best to treat it as fatal
			// rather than restarting blindly.
			proc.stop()
			if waitErr == nil {
				waitErr = fmt.Errorf("ssh exited silently")
			} else {
				waitErr = fmt.Errorf("ssh exited silently: %v", waitErr)
			}
			fwd.Kill(waitErr)
			return
		}
		proc.stop()
		var err error
		time.Sleep(sshRetryInterval)
		startTime = time.Now()
		proc, err = fwd.start()
		if err != nil {
			fwd.Kill(err)
			return
		}
	}
}

// start starts an ssh client to forward connections
// from a local port to the remote port.
func (fwd *sshForwarder) start() (p *sshProc, err error) {
	defer errorContextf(&err, "cannot start SSH client")
	args := []string{
		"-T",
		"-N",
		"-o", "StrictHostKeyChecking no",
		"-o", "PasswordAuthentication no",
		"-L", fmt.Sprintf(fmt.Sprintf("%s:localhost:%s", fwd.localAddr, fwd.remotePort)),
		"-p", fmt.Sprint(sshRemotePort),
	}
	if fwd.keyFile != "" {
		args = append(args, "-i", fwd.keyFile)
	}
	args = append(args, sshUser+fwd.remoteHost)

	c := exec.Command("ssh", args...)
	log.Printf("state: starting ssh client: %q", c.Args)
	output, err := c.StdoutPipe()
	if err != nil {
		return nil, err
	}
	c.Stderr = c.Stdout

	err = c.Start()
	if err != nil {
		return nil, err
	}

	wait := make(chan error, 1)
	go func() {
		wait <- c.Wait()
	}()

	errorc := make(chan *sshError, 1)
	go func() {
		r := bufio.NewReader(output)
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				// Discard any error, because it's likely to be
				// from the output being closed, and we can't do
				// much about it anyway - we're more interested
				// in the ssh exit status.
				return
			}
			if err := parseSSHError(strings.TrimRight(line, "\r\n")); err != nil {
				errorc <- err
				return
			}
		}
	}()

	return &sshProc{
		error:  errorc,
		wait:   wait,
		output: output,
		cmd:    c,
	}, nil
}

// sshProc represents a running ssh process.
type sshProc struct {
	error  <-chan *sshError // Error printed by ssh.
	wait   <-chan error     // SSH exit status.
	output io.Closer        // The output pipe.
	cmd    *exec.Cmd        // The running command. 
}

func (p *sshProc) stop() {
	p.output.Close()
	p.cmd.Process.Kill()
}

// sshError represents an error printed by ssh.
type sshError struct {
	fatal   bool // If true, there's no point in retrying.
	unknown bool // Whether we've failed to recognise the error message.
	msg     string
}

func (e *sshError) Error() string {
	return "ssh: " + e.msg
}

// parseSSHError parses an error as printed by ssh.
// If it's not actually an error, it returns nil.
func parseSSHError(s string) *sshError {
	if strings.HasPrefix(s, "ssh: ") {
		s = s[len("ssh: "):]
	}
	log.Printf("state: ssh: %s", s)
	err := &sshError{msg: s}
	switch {
	case strings.HasPrefix(s, "Warning: Permanently added"):
		// Even with a null host file, and ignoring strict host checking
		// we'll end up with a "Permanently added" warning.
		// suppress it as it's effectively normal for our usage...
		return nil

	case strings.HasPrefix(s, "Could not resolve hostname"):
		err.fatal = true

	case strings.Contains(s, "Permission denied"):
		err.fatal = true

	default:
		err.unknown = true
	}
	return err
}

func chooseZkPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
