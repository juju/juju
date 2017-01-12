#!/bin/bash
source $HOME/cloud-city/juju-qa.jujuci
set -eux
: ${SCRIPTS=$(readlink -f $(dirname $0))}

# The maas 1.8 on finfolk is shutdown.
#ssh -i $HOME/cloud-city/staging-juju-rsa maas-1-8 "~/clean_leaked_leases.bash"

# Release all allocated machines on maas

ssh -i $HOME/cloud-city/staging-juju-rsa jenkins@munna <<"EOT"
    set -eux
    for mid in $(maas 210-maas nodes list-allocated | egrep '(system_id)' | sed -r 's,.*"([^"]*)".*,\1,'); do
        maas 210-maas node release $mid;
    done
EOT

ssh -i $HOME/cloud-city/staging-juju-rsa jenkins@finfolk <<"EOT"
    set -eux
    for mid in $(maas env20 machines list-allocated | egrep '(system_id)' | sed -r 's,.*"([^"]*)".*,\1,'); do
        maas env20 machine release $mid;
    done
EOT

ssh -i $HOME/cloud-city/staging-juju-rsa jenkins@silcoon <<"EOT"
    set -eux
    for mid in $(maas env21 machines list-allocated | egrep '(system_id)' | sed -r 's,.*"([^"]*)".*,\1,'); do
        maas env21 machine release $mid;
    done
EOT

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
