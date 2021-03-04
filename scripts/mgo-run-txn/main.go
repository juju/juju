// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"time"

	"github.com/juju/clock"
	"github.com/juju/gnuflag"
	"github.com/juju/mgo/v2"
	"github.com/juju/mgo/v2/bson"
	"github.com/juju/mgo/v2/txn"
	jujutxn "github.com/juju/txn"
	"github.com/kr/pretty"
)

type Args struct {
	MongoHost    string
	Username     string
	Password     string
	Database     string
	AuthDatabase string
	SSL          bool
	Verbose      bool
}

var defaultArgs = Args{
	MongoHost:    "localhost:37017",
	Username:     "admin",
	Password:     "",
	Database:     "juju",
	AuthDatabase: "admin",
	SSL:          true,
	Verbose:      false,
}

// Operation matches a txn.Op but is designed around deserializing from JSON rather than just-anything-you-can-put-in-BSON
type Operation struct {
	Collection string      `json:"c"`
	Document   interface{} `json:"d"` // Could be a string, int or nested doc?
	Assertion  interface{} `json:"a"` // do we want to support d- / d+ ?
	Insert     *bson.M     `json:"i"` // could be bson.D but M feels better here
	Update     *bson.M     `json:"u"` // similarly could be bson.D
	Remove     bool        `bson:"r"`
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
	flags.StringVar(&args.Username, "user", defaultArgs.Username, "username to connect as")
	flags.StringVar(&args.Password, "password", defaultArgs.Password, "password for connection")
	flags.StringVar(&args.Database, "db", defaultArgs.Database, "database to access")
	flags.StringVar(&args.AuthDatabase, "authdb", defaultArgs.AuthDatabase, "database to use for authentication")
	flags.BoolVar(&args.SSL, "ssl", defaultArgs.SSL, "use --ssl=false to disable ssl")
	flags.BoolVar(&args.Verbose, "v", defaultArgs.Verbose, "print transaction before running it")
	return args
}

func dialSSL(addr *mgo.ServerAddr) (net.Conn, error) {
	c, err := net.Dial("tcp", addr.String())
	if err != nil {
		return nil, err
	}
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}
	cc := tls.Client(c, tlsConfig)
	if err := cc.Handshake(); err != nil {
		return nil, err
	}
	return cc, nil
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
	bytes, err := ioutil.ReadAll(f)
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
	if args.SSL {
		dialInfo.DialServer = dialSSL
	}
	session, err := mgo.DialWithInfo(dialInfo)
	if err != nil {
		fmt.Printf("error connecting to mongo:\n%v\n", err)
		os.Exit(1)
	}
	runner := jujutxn.NewRunner(jujutxn.RunnerParams{
		Database:                  session.DB(args.Database),
		TransactionCollectionName: "txns",
		ChangeLogName:             "txns.log",
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
