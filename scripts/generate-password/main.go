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
	"github.com/juju/utils/v3"
)

func main() {
	gnuflag.Usage = func() {
		_, _ = fmt.Fprintf(os.Stderr, "Usage: %s <modeluuid> <agent> [<password>] | --user <username> [password]\n", os.Args[0])
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
			passwd, err = utils.RandomPassword()
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
		agentType, collection := classifyTarget(agent)
		hash := utils.AgentPasswordHash(passwd)
		fmt.Printf("oldpassword: %s\n", passwd)
		fmt.Printf(`db.%s.update({"_id": "%s:%s"}, {$set: {"passwordhash": "%s"}})`+"\n",
			collection, modelUUID, agent, hash)
		if agentType == targetK8sApplicationAgent {
			printK8sApplicationAgentHelp(agent)
		}
	}
}

type targetType int

const (
	targetMachine targetType = iota
	targetUnit
	targetK8sApplicationAgent
)

func classifyTarget(target string) (targetType, string) {
	if strings.Contains(target, "/") {
		return targetUnit, "units"
	}
	if _, err := strconv.Atoi(target); err == nil {
		return targetMachine, "machines"
	}
	// Bare non-integer, non-unit identifiers are treated as CAAS application
	// names for password-hash recovery.
	return targetK8sApplicationAgent, "applications"
}

func printK8sApplicationAgentHelp(appName string) {
	fmt.Printf("\nKubernetes application-agent recovery:\n")
	fmt.Printf("1. Update the introduction secret for new pod init.\n")
	fmt.Printf(
		`kubectl -n <model-namespace> patch secret %s-application-config --type merge -p '{"stringData":{"JUJU_K8S_APPLICATION_PASSWORD":"<password>"}}'`+"\n",
		appName,
	)
	fmt.Printf("2. Restart workload pods so init picks up the new secret.\n")
	fmt.Printf(`kubectl -n <model-namespace> delete pod -l app.kubernetes.io/name=%s`+"\n", appName)
}
