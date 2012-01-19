package main

import "launchpad.net/juju/go/control"
import "log"
import "os"

func main() {
    if err := control.JujuMain(os.Args); err != nil {
        log.Println(err)
        os.Exit(1)
    }
}
