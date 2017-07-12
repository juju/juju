#!/bin/bash

dialmgo() {
  agent=$(cd /var/lib/juju/agents; echo machine-*)
  pw=$(sudo cat /var/lib/juju/agents/${agent}/agent.conf |grep statepassword |awk '{ print $2 }')
  /usr/lib/juju/mongo3.2/bin/mongo --ssl --sslAllowInvalidCertificates -u ${agent} -p $pw localhost:37017/juju --authenticationDatabase admin
}

