package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"

	simpleapi "github.com/juju/juju/grpc/gen/proto/go/juju/client/simple/v1alpha1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

func main() {
	var (
		certFile       string
		username       string
		password       string
		controllerHost string
		modelUUID      string
	)
	flag.StringVar(&certFile, "cacert", "cacert.pem", "path to cacert pem file")
	flag.StringVar(&username, "u", "admin", "username")
	flag.StringVar(&password, "p", "", "password")
	flag.StringVar(&controllerHost, "controller", "", "controller hostname")
	flag.StringVar(&modelUUID, "model", "", "model UUID")
	flag.Parse()

	ctx := context.Background()
	creds, err := credentials.NewClientTLSFromFile(certFile, "juju-apiserver")
	if err != nil {
		log.Fatal(err)
	}
	conn, err := grpc.DialContext(ctx, controllerHost+":18888", grpc.WithTransportCredentials(creds))
	if err != nil {
		log.Fatal(err)
	}
	client := simpleapi.NewSimpleServiceClient(conn)

	ctx = metadata.AppendToOutgoingContext(ctx,
		"authorization", fmt.Sprintf("basic %s:%s", username, password),
		"model-uuid", modelUUID,
	)
	resp, err := client.Status(ctx, &simpleapi.StatusRequest{})
	if err != nil {
		log.Fatal(err)
	}
	b, err := json.MarshalIndent(resp.Model, "  ", "  ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s\n", b)
}
