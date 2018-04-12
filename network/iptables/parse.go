// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build ignore

package main

import (
	"log"
	"os"

	"github.com/kr/pretty"

	"github.com/juju/juju/network/iptables"
)

func main() {
	rules, err := iptables.ParseIngressRules(os.Stdin)
	if err != nil {
		log.Fatal(err)
	}
	pretty.Println(rules)
}
