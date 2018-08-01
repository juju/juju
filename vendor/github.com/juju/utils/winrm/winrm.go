// Copyright 2016 Canonical ltd.
// Copyright 2016 Cloudbase solutions
// Licensed under the lgplv3, see licence file for details.

package winrm

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/masterzen/winrm"
	"golang.org/x/crypto/ssh/terminal"
)

const (
	httpPort            = 5985
	httpsPort           = 5986
	defaultWinndowsUser = "Administrator"
)

var (
	logger = loggo.GetLogger("juju.utils.winrm")
)

// Client type retains information about the winrm connection
type Client struct {
	conn   *winrm.Client
	pass   string
	secure bool
}

// Secure returns true if the client is using a secure connection or false
// if it's just a normal http
func (c Client) Secure() bool {
	return c.secure
}

// Password returns the winrm connection password
func (c Client) Password() string {
	return c.pass
}

// ClientConfig used for setting up a secure https client connection
type ClientConfig struct {
	// User of the client connection
	// If you want the default user Administrator, leave this empty
	User string
	// Host where we want to connect
	Host string
	// Key Private RSA key
	Key []byte
	// Cert https client x509 cert
	Cert []byte
	// CACert of the host we wish to connect
	// This can be nil if Insecure is false
	CACert []byte
	// Timeout on how long we should wait until a connection is made.
	// If empty this will use the 60*time.Seconds winrm default value
	Timeout time.Duration
	// Insecure flag for checking the CACert or not. If this is true there should
	// be a valid CACert passed also.
	Insecure bool
	// Password callback for returning a password in different ways
	Password GetPasswd
	// Secure flag for specifying if the user wants https or http
	Secure bool
}

// errNoPasswdFn sentinel for returning unset password callback
var errNoPasswdFn = fmt.Errorf("No password callback")

// password checks if the password callback is set and returns the password
// from that specific get password handler
func (c ClientConfig) password() (string, error) {
	if c.Password != nil {
		pass, err := c.Password()
		return pass, err
	}
	return "", errNoPasswdFn
}

// GetPasswd callback for different semantics that a client
// could use for secure authentication
type GetPasswd func() (string, error)

// TTYGetPasswd will be valid if it's used from a valid TTY input,
// This can be passed in ClientConfig
func TTYGetPasswd() (string, error) {
	// make it look like a dropdown shell
	fmt.Fprintf(os.Stdout, "[Winrm] >> ")
	// very important that os.Stdin.Fd() needs to be a valid tty.
	// if the file descriptor dosen't point to a terminal this will fail
	// with the message inappropriate ioctl for device
	pass, err := terminal.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintf(os.Stdout, "\n")
	if err != nil {
		return "", errors.Annotatef(err, "Can't retrive password from terminal")
	}
	return string(pass), nil
}

// Validate checks all config options if they are invalid
func (c ClientConfig) Validate() error {
	if c.Host == "" {
		return fmt.Errorf("Empty host in client config")
	}

	// if the connection is https
	if c.Secure == true {
		// check if we we or not the password authentication method
		if c.Password == nil {
			// that meas we need to be sure if the cert and key is set
			// for cert authentication
			if c.Key == nil || c.Cert == nil {
				return fmt.Errorf("Empty key or cert in client config")
			}
			// everything is set
			logger.Infof("using https winrm connection with cert authentication")
		} else { // we are using https with password authentication
			logger.Infof("Using https winrm connection with password authentication")
		}
		// extra check if we are dealing with CA cert skip ornot
		if !c.Insecure {
			// if the Insecure is not set then we must check if
			// ca is set also
			if c.CACert == nil {
				return fmt.Errorf("Empty CA cert passed in client config")
			}
			// we are using Insecure option so we should skip the CA verification
		} else {
			logger.Warningf("Skipping CA server verification, using Insecure option")
		}
		// we are using http connection so we can only authenticate using password auth method
	} else {
		// if the password is not set
		if c.Password == nil {
			if c.Key != nil || c.Cert != nil {
				return fmt.Errorf("Cannot use cert auth with http connection")
			}
			return fmt.Errorf("Nil password getter, unable to retrive password")
		}
		// the password is set so we are good.
		logger.Infof("Using http winrm connection with password authentication")
	}

	return nil
}

// NewClient creates a new secure winrm client for initiating connections with the winrm listener
func NewClient(config ClientConfig) (*Client, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Annotatef(err, "cannot create winrm client")
	}

	cli := &Client{}
	params := winrm.NewParameters("PT60S", "en-US", 153600)
	var err error
	cli.pass, err = config.password()
	if err != errNoPasswdFn && err != nil {
		return nil, errors.Annotatef(err, "cannot get password")
	}

	// if we didn't provided a callback password that means
	// we want to use the https auth method that only works with https
	if err == errNoPasswdFn && config.Secure {
		// when creating a new client the winrm.DeafultParameters
		// will be used to make a new client conneciton to a endpoint
		// TransportDecorator will enable us to switch transports
		// this will be used for https client x509 authentication
		params.TransportDecorator = func() winrm.Transporter {
			logger.Debugf("Switching WinRM transport to HTTPS")
			// winrm https module
			return &winrm.ClientAuthRequest{}
		}
	}

	port := httpPort
	cli.secure = false
	if config.Secure {
		port = httpsPort
		cli.secure = true
	}

	endpoint := winrm.NewEndpoint(config.Host, port,
		config.Secure, config.Insecure,
		config.CACert, config.Cert,
		config.Key, config.Timeout,
	)

	// if the user is empty
	if config.User == "" {
		// use the default one, Administrator
		config.User = defaultWinndowsUser
	}

	cli.conn, err = winrm.NewClientWithParameters(endpoint, config.User, cli.pass, params)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot create WinRM https client conn")
	}
	return cli, nil
}

// Run powershell command and direct output to stdout and errors to stderr
// If the Run successfully executes it returns nil
func (c *Client) Run(command string, stdout io.Writer, stderr io.Writer) error {
	logger.Debugf("Runing cmd on WinRM connection %q", command)
	_, err := c.conn.Run(command, stdout, stderr)
	if err != nil {
		return errors.Annotatef(err, "cannot run WinRM command %q", command)
	}
	return nil
}

var (
	// ErrAuth returned if the post request is droped due to invalid credentials
	ErrAuth = fmt.Errorf("Unauthorized request")
	// ErrPing returned if the ping fails
	ErrPing = fmt.Errorf("Ping failed, can't recive any response form target")
)

// Ping executes a simple echo command on the remote, if the server dosen't respond
// to this echo command it will return ErrPing. If the payload is executed and the server
// response accordingly we return nil
func (c *Client) Ping() error {
	const echoPayload = "HI!"

	logger.Debugf("Pinging WinRM listener")

	var stdout, stderr bytes.Buffer
	if err := c.Run("ECHO "+echoPayload, &stdout, &stderr); err != nil {
		// we should check if winrm returned 401
		// to know if we have permision to access the remote.
		if strings.Contains(err.Error(), "401") {
			logger.Warningf("%s", "The machine blocked due to unathorized access")
			return ErrAuth
		}
		logger.Errorf("%s", err)
		return ErrPing
	}

	if stderr.Len() != 0 {
		return fmt.Errorf("command failed with %s",
			strings.TrimSpace(stderr.String()),
		)
	}

	logger.Debugf("Output of the ping matches: %s", strconv.FormatBool(strings.Contains(stdout.String(), "HI!")))
	if stdout.String() == echoPayload {
		return errors.Errorf("unexpected response: expected %q, got %q", echoPayload, stdout.String())
	}

	return nil
}
