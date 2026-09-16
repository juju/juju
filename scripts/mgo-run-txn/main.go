// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/gnuflag"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	jujutxn "github.com/juju/txn/v3"
	"github.com/kr/pretty"
)

type Args struct {
	MongoHost     string
	Username      string
	Password      string
	Database      string
	AuthDatabase  string
	TLS           bool
	TLSCAFile     string
	TLSCertFile   string
	TLSServerName string
	TLSInsecure   bool
	Legacy        bool
	Verbose       bool
}

var defaultArgs = Args{
	MongoHost:     "localhost:37017",
	Username:      "",
	Password:      "",
	Database:      "juju",
	AuthDatabase:  "admin",
	TLS:           true,
	TLSServerName: "juju-mongodb",
	Verbose:       false,
}

// Default snap paths used by juju-db 4.4.30+ on the controller host.
const (
	defaultSnapCAFile   = "/var/snap/juju-db/common/ca.crt"
	defaultSnapCertFile = "/var/snap/juju-db/common/server.pem"
)

// Operation matches a txn.Op but is designed around deserializing from JSON rather than just-anything-you-can-put-in-BSON
type Operation struct {
	Collection string      `json:"c"`
	Document   interface{} `json:"d"` // Could be a string, int or nested doc?
	Assertion  interface{} `json:"a"` // do we want to support d- / d+ ?
	Insert     *bson.M     `json:"i"` // could be bson.D but M feels better here
	Update     *bson.M     `json:"u"` // similarly could be bson.D
	Remove     bool        `json:"r"`
}

func setupArgs(flags *gnuflag.FlagSet) *Args {
	args := &Args{}

	flags.Usage = func() {
		fmt.Printf(`
Usage: %s txn-file

Where txn is a JSON encoded list of transaction operations taking the form:
[{
  "c": "collection",
  "d": "document ID",
  "a": "assertions",
  "i": "document to Insert",
  "u": "updates to Document",
  "r": "boolean true to remove"
}]
`[1:], os.Args[0])
		flags.PrintDefaults()
	}
	flags.StringVar(&args.MongoHost, "host", defaultArgs.MongoHost, "host[:port] to connect to")
	flags.StringVar(&args.Username, "user", defaultArgs.Username, "username to connect as (defaults to the machine tag from agent.conf on the controller)")
	flags.StringVar(&args.Password, "password", defaultArgs.Password, "password for connection (defaults to the state password from agent.conf on the controller)")
	flags.StringVar(&args.Database, "db", defaultArgs.Database, "database to access")
	flags.StringVar(&args.AuthDatabase, "authdb", defaultArgs.AuthDatabase, "database to use for authentication")
	flags.BoolVar(&args.TLS, "tls", defaultArgs.TLS, "use --tls=false to disable tls")
	flags.StringVar(&args.TLSCAFile, "ca-file", defaultSnapCAFile, "CA certificate file (PEM) used to verify the server")
	flags.StringVar(&args.TLSCertFile, "cert-file", "", "client certificate and key file (PEM) to present to the server")
	flags.StringVar(&args.TLSServerName, "server-name", defaultArgs.TLSServerName, "TLS server name to verify against")
	flags.BoolVar(&args.TLSInsecure, "tls-insecure", false, "skip server certificate verification (default when no --ca-file is given)")
	flags.BoolVar(&args.Legacy, "legacy", false, "connect without the juju-db 4.4.30+ client certificate (for older mongos)")
	flags.BoolVar(&args.Verbose, "v", defaultArgs.Verbose, "print transaction before running it")
	return args
}

// buildTLSConfig returns the tls.Config used for the mongo connection.
// Against modern juju-db (4.4.30+) the server requires a client
// certificate signed by the controller CA. By default both files come
// from the juju-db snap on the controller host:
//
//	/var/snap/juju-db/common/ca.crt
//	/var/snap/juju-db/common/server.pem
func buildTLSConfig(args *Args) (*tls.Config, error) {
	if args.Legacy {
		return &tls.Config{InsecureSkipVerify: true}, nil
	}
	useSnap := args.TLSCAFile == defaultSnapCAFile && args.TLSCertFile == ""
	caFile, certFile := args.TLSCAFile, args.TLSCertFile
	rootHint := "; the snap files are root-owned, run as root or with `sudo -n` (or pass --legacy for pre-4.4.30)"
	if useSnap {
		// Running on a controller with the juju-db snap: present the
		// shared server certificate so the 4.4.30+ mutual-TLS
		// requirement is satisfied out of the box.
		if _, err := os.Stat(defaultSnapCAFile); err != nil {
			return nil, fmt.Errorf("default CA file %q not found (unreadable without root?)%s", defaultSnapCAFile, rootHint)
		}
		if _, err := os.Stat(defaultSnapCertFile); err != nil {
			return nil, fmt.Errorf("default client certificate file %q not found (unreadable without root?)%s", defaultSnapCertFile, rootHint)
		}
		certFile = defaultSnapCertFile
	}
	pem, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("reading CA file: %v%s", err, rootHint)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("no certificates found in CA file %q", caFile)
	}
	cfg := &tls.Config{
		RootCAs:    pool,
		ServerName: args.TLSServerName,
	}
	if certFile != "" {
		srvPEM, err := os.ReadFile(certFile)
		if err != nil {
			return nil, fmt.Errorf("reading certificate file: %v%s", err, rootHint)
		}
		cert, err := tls.X509KeyPair(srvPEM, srvPEM)
		if err != nil {
			return nil, fmt.Errorf("parsing certificate file: %v", err)
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	return cfg, nil
}

func dialTLS(addr *mgo.ServerAddr, cfg *tls.Config) (net.Conn, error) {
	c, err := net.Dial("tcp", addr.String())
	if err != nil {
		return nil, err
	}
	cc := tls.Client(c, cfg)
	if err := cc.Handshake(); err != nil {
		return nil, err
	}
	return cc, nil
}

// agentConfGlob matches the machine agent configuration files written by
// juju on the controller host.
const agentConfGlob = "/var/lib/juju/agents/machine-*/agent.conf"

// readAgentConf reads the mongo credentials (machine tag and state
// password) from a local juju machine agent.conf, so the tool can run on
// a controller without requiring --user/--password. The file is
// root-owned; run the tool as root (or with `sudo -n`) to read it.
func readAgentConf() (user, password string, err error) {
	matches, gerr := filepath.Glob(agentConfGlob)
	if gerr != nil {
		return "", "", gerr
	}
	if len(matches) == 0 {
		return "", "", fmt.Errorf("no agent.conf files match %q", agentConfGlob)
	}
	var readErr error
	for _, path := range matches {
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			readErr = rerr
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			switch {
			case strings.HasPrefix(line, "tag:"):
				if user == "" {
					user = strings.TrimSpace(strings.TrimPrefix(line, "tag:"))
				}
			case strings.HasPrefix(line, "statepassword:"):
				if password == "" {
					password = strings.TrimSpace(strings.TrimPrefix(line, "statepassword:"))
				}
			}
		}
		if user != "" && password != "" {
			return user, password, nil
		}
	}
	if readErr == nil {
		readErr = fmt.Errorf("no tag/statepassword found in %v", matches)
	}
	if user == "" && password == "" {
		return "", "", readErr
	}
	// Partial parse: return what we have, the caller will fill the rest.
	return user, password, nil
}

func main() {
	flags := gnuflag.NewFlagSet(os.Args[0], gnuflag.ExitOnError)
	args := setupArgs(flags)
	if err := flags.Parse(true, os.Args[1:]); err != nil {
		fmt.Printf("error parsing arguments:\n%s\n", err)
		os.Exit(1)
	}
	if flags.NArg() < 1 {
		flags.Usage()
		os.Exit(1)
	}
	f, err := os.Open(flags.Arg(0))
	if err != nil {
		fmt.Printf("error opening file %s:\n%s\n", flags.Arg(0), err)
		os.Exit(1)
	}
	bytes, err := io.ReadAll(f)
	if err != nil {
		fmt.Printf("error reading file %s:\n%s\n", flags.Arg(0), err)
		os.Exit(1)
	}
	var ops []Operation
	if err := json.Unmarshal(bytes, &ops); err != nil {
		fmt.Printf("error parsing transaction operations:\n%s\n", err)
		os.Exit(1)
	}
	if args.Verbose {
		fmt.Printf("Parsed transaction:\n%s\n", pretty.Sprint(ops))
	}

	tlsConfig, err := buildTLSConfig(args)
	if err != nil {
		fmt.Printf("error building TLS config:\n%v\n", err)
		os.Exit(1)
	}

	if args.Username == "" || args.Password == "" {
		user, pass, aerr := readAgentConf()
		if aerr != nil {
			fmt.Printf("error reading machine agent.conf for mongo credentials: %v\n", aerr)
			fmt.Printf("the agent.conf is root-owned; run as root or with `sudo -n`, or pass --user and --password explicitly\n")
			os.Exit(1)
		}
		if args.Username == "" {
			args.Username = user
		}
		if args.Password == "" {
			args.Password = pass
		}
	}
	if args.Password == "" {
		fmt.Printf("no mongo password available; pass --password\n")
		os.Exit(1)
	}

	dialInfo := &mgo.DialInfo{
		Addrs:      []string{args.MongoHost},
		Direct:     true,
		Timeout:    time.Second,
		Database:   args.Database,
		Source:     args.AuthDatabase,
		Username:   args.Username,
		Password:   args.Password,
		DialServer: nil, // func(addr *ServerAddr) (net.Conn, error)
	}
	if args.TLS {
		server := tlsConfig
		dialInfo.DialServer = func(addr *mgo.ServerAddr) (net.Conn, error) {
			return dialTLS(addr, server)
		}
	}
	session, err := mgo.DialWithInfo(dialInfo)
	if err != nil {
		fmt.Printf("error connecting to mongo:\n%v\n", err)
		os.Exit(1)
	}
	runner := jujutxn.NewRunner(jujutxn.RunnerParams{
		Database:                  session.DB(args.Database),
		TransactionCollectionName: "txns",
		ChangeLogName:             "-",
		ServerSideTransactions:    true,
		Clock:                     clock.WallClock,
	})
	txnOps := make([]txn.Op, len(ops))
	for i, o := range ops {
		op := txn.Op{
			C:      o.Collection,
			Id:     o.Document,
			Remove: o.Remove,
		}
		if o.Assertion != nil {
			switch a := o.Assertion.(type) {
			case string:
				op.Assert = a
			case map[string]interface{}:
				op.Assert = bson.M(a)
			default:
				fmt.Printf("unknown Assertion: %v\n", o.Assertion)
				os.Exit(1)
			}
		}
		if o.Insert != nil {
			op.Insert = *o.Insert
		}
		if o.Update != nil {
			op.Update = *o.Update
		}
		txnOps[i] = op
	}
	transaction := jujutxn.Transaction{
		Ops: txnOps,
	}
	if err := runner.RunTransaction(&transaction); err != nil {
		fmt.Printf("error running transaction:\n%v\n", err)
		os.Exit(2)
	} else {
		fmt.Printf("success\n")
	}
	return
}
