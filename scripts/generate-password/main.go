// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/juju/gnuflag"
	"github.com/juju/utils"
)

func main() {
	gnuflag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [modeluuid agent|--user <username>]\n", os.Args[0])
		gnuflag.PrintDefaults()
	}
	user := gnuflag.String("user", "", "supply a username to generate a password instead of modeluuid and agent")
	gnuflag.Parse(true)
	args := gnuflag.Args()
	var modelUUID string
	var agent string
	if *user == "" {
		if len(args) < 2 {
			gnuflag.Usage()
			os.Exit(1)
		}
		modelUUID = args[0]
		agent = args[1]
	}
	passwd, err := utils.RandomPassword()
	if err != nil {
		log.Fatal(err)
	}
	if *user != "" {
		salt, err := utils.RandomSalt()
		if err != nil {
			log.Fatal(err)
		}
		hash := utils.UserPasswordHash(passwd, salt)
		fmt.Printf("Password line for ~/.local/share/juju/accounts.yaml\n")
		fmt.Printf("  password: %s\n", passwd)
		fmt.Printf(`db.users.update({"_id": "%s"}, {"$set": {"passwordsalt": "%s", "passwordhash": "%s"}})`+"\n",
			*user, salt, hash)
	} else {
		hash := utils.AgentPasswordHash(passwd)
		fmt.Printf("oldpassword: %s\n", passwd)
		collection := "UNKNOWN"
		if strings.Index(agent, "/") < 0 {
			// must be a machine
			collection = "machines"
		} else {
			collection = "units"
		}
		fmt.Printf(`db.%s.update({"_id": "%s:%s"}, {$set: {"passwordhash": "%s"}})`+"\n",
			collection, modelUUID, agent, hash)
	}
}
