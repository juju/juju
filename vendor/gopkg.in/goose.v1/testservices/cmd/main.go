package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"launchpad.net/gnuflag"

	"gopkg.in/goose.v1/testservices/identityservice"
)

type userInfo struct {
	user, secret string
}
type userValues struct {
	users []userInfo
}

func (uv *userValues) Set(s string) error {
	vals := strings.Split(s, ":")
	if len(vals) != 2 {
		return fmt.Errorf("Invalid --user option, should be: user:secret")
	}
	uv.users = append(uv.users, userInfo{
		user:   vals[0],
		secret: vals[1],
	})
	return nil
}
func (uv *userValues) String() string {
	return fmt.Sprintf("%v", uv.users)
}

var provider = gnuflag.String("provider", "userpass", "provide the name of the identity service to run")

var serveAddr = gnuflag.String("addr", "localhost:8080", "serve the provider on the given address.")

var users userValues

func init() {
	gnuflag.Var(&users, "user", "supply to add a user to the identity provider. Can be supplied multiple times. Should be of the form \"user:secret:token\".")
}

var providerMap = map[string]identityservice.IdentityService{
	"legacy":   identityservice.NewLegacy(),
	"userpass": identityservice.NewUserPass(),
}

func providers() []string {
	out := make([]string, 0, len(providerMap))
	for provider := range providerMap {
		out = append(out, provider)
	}
	return out
}

func main() {
	gnuflag.Parse(true)
	p, ok := providerMap[*provider]
	if !ok {
		log.Fatalf("No such provider: %s, pick one of: %v", *provider, providers())
	}
	mux := http.NewServeMux()
	p.SetupHTTP(mux)
	for _, u := range users.users {
		p.AddUser(u.user, u.secret, "tenant")
	}
	log.Fatal(http.ListenAndServe(*serveAddr, mux))
}
