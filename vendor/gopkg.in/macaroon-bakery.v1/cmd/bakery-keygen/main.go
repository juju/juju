package main

import (
	"encoding/json"
	"fmt"
	"os"

	"gopkg.in/macaroon-bakery.v1/bakery"
)

func main() {
	kp, err := bakery.GenerateKey()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot generate key: %s\n", err)
		os.Exit(1)
	}
	b, err := json.MarshalIndent(kp, "", "\t")
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s\n", b)
}
