// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/juju/gnuflag"
	"github.com/juju/utils/v3"
)

func main() {
	gnuflag.Usage = func() {
		_, _ = fmt.Fprintf(os.Stderr, "Usage: %s [--model-name <modelname>] <modeluuid> <agent> [<password>] | --user <username> [password]\n", os.Args[0])
		gnuflag.PrintDefaults()
	}
	user := gnuflag.String("user", "", "supply a username to generate a password instead of modeluuid and agent")
	modelName := gnuflag.String("model-name", "", "Kubernetes model name, used as the namespace for application-agent recovery")
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
		if agentType == targetK8sApplicationAgent && *modelName == "" {
			_, _ = fmt.Fprintln(os.Stderr, "--model-name is required for Kubernetes application-agent recovery")
			os.Exit(1)
		}
		hash := utils.AgentPasswordHash(passwd)
		fmt.Printf("oldpassword: %s\n", passwd)
		fmt.Printf(`db.%s.update({"_id": "%s:%s"}, {$set: {"passwordhash": "%s"}})`+"\n",
			collection, modelUUID, agent, hash)
		if agentType == targetK8sApplicationAgent {
			if err := printK8sApplicationAgentHelp(os.Stdout, *modelName, agent, passwd); err != nil {
				log.Fatal(err)
			}
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

func printK8sApplicationAgentHelp(w io.Writer, modelName, appName, password string) error {
	passwordBase64 := base64.StdEncoding.EncodeToString([]byte(password))
	_, err := fmt.Fprintf(w, `
Kubernetes application-agent recovery:
1. Update the introduction secret for new pod init.
kubectl -n %s patch secret %s-application-config --type merge -p '{"data":{"JUJU_K8S_APPLICATION_PASSWORD":"%s"}}'
2. Restart workload pods so init picks up the new secret.
kubectl -n %s delete pod -l app.kubernetes.io/name=%s
`, modelName, appName, passwordBase64, modelName, appName)
	return err
}
