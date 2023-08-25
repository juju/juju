// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build ignore

package main

import (
	"log"
	"os"

	"github.com/kr/pretty"

	"github.com/juju/juju/internal/network/iptables"
)

func main() {
	rules, err := iptables.ParseIngressRules(os.Stdin)
	if err != nil {
		log.Fatal(err)
	}
	pretty.Println(rules)
}
