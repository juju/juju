package juju

import "launchpad.net/juju/go/environs"

type Conn struct {
    environ environs.Environ
}

func NewConn(environ_name string) (*Conn, error) {
    environs, err := environs.ReadEnvirons("")
    if err != nil {
        return nil, err
    }
    environ, err := environs.Open(environ_name)
    if err != nil {
        return nil, err
    }
    return &Conn{environ}, nil
}
