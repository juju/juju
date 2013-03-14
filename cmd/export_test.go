package cmd

import (
	stdlog "log"
	"net"
	"time"
)

func runSyslog(c net.PacketConn, done chan<- string) {
	var buf [4096]byte
	var rcvd string = ""
	for {
		n, _, err := c.ReadFrom(buf[0:])
		if err != nil || n == 0 {
			break
		}
		rcvd += string(buf[0:n])
	}
	done <- rcvd
}

func StartTestSysLogServer(done chan<- string) string {
	c, e := net.ListenPacket("udp", "127.0.0.1:0")
	if e != nil {
		stdlog.Fatalf("net.ListenPacket failed udp :0 %v", e)
	}
	c.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	go runSyslog(c, done)
	return c.LocalAddr().String()
}
