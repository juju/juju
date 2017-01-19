#!/bin/bash
source $HOME/cloud-city/juju-qa.jujuci
set -eux
: ${SCRIPTS=$(readlink -f $(dirname $0))}

# The maas 1.8 on finfolk is shutdown.
#ssh -i $HOME/cloud-city/staging-juju-rsa maas-1-8 "~/clean_leaked_leases.bash"

# Release all allocated machines on maas

ssh -i $HOME/cloud-city/staging-juju-rsa jenkins@munna \
  'JUJU_HOME=~/cloud-city' juju-ci-tools/clean_maas.py parallel-munna-vmaas \
  --hours=2

ssh -i $HOME/cloud-city/staging-juju-rsa jenkins@finfolk \
  'JUJU_HOME=~/cloud-city' juju-ci-tools/clean_maas.py parallel-finfolk-vmaas \
  --hours=2

ssh -i $HOME/cloud-city/staging-juju-rsa jenkins@silcoon \
  'JUJU_HOME=~/cloud-city' juju-ci-tools/clean_maas.py parallel-silcoon-vmaas \
  --hours=2

# Delete all lxd containers left behind on several machines.

ssh -i $HOME/cloud-city/staging-juju-rsa jenkins@silcoon \
    juju-ci-tools/clean_lxd.py

ssh -i $HOME/cloud-city/staging-juju-rsa jenkins@feature-slave.vapour.ws \
    juju-ci-tools/clean_lxd.py

ssh -i $HOME/cloud-city/staging-juju-rsa jenkins@lxd-slave-a.vapour.ws \
    juju-ci-tools/clean_lxd.py

ssh -i $HOME/cloud-city/staging-juju-rsa jenkins@lxd-slave-b.vapour.ws \
    juju-ci-tools/clean_lxd.py

ssh -i $HOME/cloud-city/staging-juju-rsa jenkins@arm64-slave \
    juju-ci-tools/clean_lxd.py

ssh -i $HOME/cloud-city/staging-juju-rsa jenkins@s390x-slave \
    juju-ci-tools/clean_lxd.py

ssh -i $HOME/cloud-city/staging-juju-rsa jenkins@xenial-slave.vapour.ws \
    juju-ci-tools/clean_lxd.py

ssh -i $HOME/cloud-city/staging-juju-rsa jenkins@yakkety-slave.vapour.ws \
    juju-ci-tools/clean_lxd.py
