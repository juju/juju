#!/bin/bash

dialmgo() {
  agent=$(cd /var/lib/juju/agents; echo machine-*)
  pw=$(sudo cat /var/lib/juju/agents/${agent}/agent.conf |grep statepassword |awk '{ print $2 }')
  /snap/bin/juju-db.mongo --tls --tlsAllowInvalidCertificates -u ${agent} -p $pw localhost:37017/juju --authenticationDatabase admin
}

