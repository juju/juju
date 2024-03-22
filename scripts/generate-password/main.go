// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/juju/gnuflag"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/internal/password"
)

func main() {
	gnuflag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s <modeluuid> <agent> [<password>] | --user <username> [password]\n", os.Args[0])
		gnuflag.PrintDefaults()
	}
	user := gnuflag.String("user", "", "supply a username to generate a password instead of modeluuid and agent")
	gnuflag.Parse(true)
	args := gnuflag.Args()
	var modelUUID string
	var agent string
	var passwd string
	if *user == "" {
		if len(args) < 2 {
			gnuflag.Usage()
			os.Exit(1)
		}
		modelUUID = args[0]
		agent = args[1]
		if len(args) > 2 {
			passwd = args[2]
		} else {
			var err error
			passwd, err = password.RandomPassword()
			if err != nil {
				log.Fatal(err)
			}
		}
	} else {
		if len(args) < 1 {
			var err error
			passwd, err = utils.RandomPassword()
			if err != nil {
				log.Fatal(err)
			}
		} else {
			passwd = args[0]
		}
	}
	if *user != "" {
		salt, err := password.RandomSalt()
		if err != nil {
			log.Fatal(err)
		}
		hash := password.UserPasswordHash(passwd, salt)
		fmt.Printf("Password line for ~/.local/share/juju/accounts.yaml\n")
		fmt.Printf("  password: %s\n", passwd)
		fmt.Printf(`db.users.update({"_id": "%s"}, {"$set": {"passwordsalt": "%s", "passwordhash": "%s"}})`+"\n",
			*user, salt, hash)
	} else {
		var collection string
		if strings.Index(agent, "/") < 0 {
			// must be a machine
			collection = "machines"
			if _, err := strconv.Atoi(agent); err != nil {
				fmt.Fprintf(os.Stderr, "Agent %q isn't a unit agent (with /) nor an integer machine id\n", agent)
				os.Exit(1)
			}
		} else {
			collection = "units"
		}
		hash := password.AgentPasswordHash(passwd)
		fmt.Printf("oldpassword: %s\n", passwd)
		fmt.Printf(`db.%s.update({"_id": "%s:%s"}, {$set: {"passwordhash": "%s"}})`+"\n",
			collection, modelUUID, agent, hash)
	}
}
