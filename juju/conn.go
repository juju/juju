package juju

import (
	"fmt"
	"launchpad.net/juju/go/environs"
)

type Conn struct {
	environ environs.Environ
}

func NewConn(environName string) (*Conn, error) {
	environs, err := environs.ReadEnvirons("")
	if err != nil {
		return nil, err
	}
	environ, err := environs.Open(environName)
	if err != nil {
		return nil, err
	}
	return &Conn{environ}, nil
}

func (c *Conn) Bootstrap() error {
	return fmt.Errorf("This doesn't do anything yet.")
}
