package main

import "launchpad.net/juju/go/control"
import "os"

func main() {
    control.JujuMainCommand().Main(os.Args)
}
