#!/bin/bash
set -eux
RELEASE=$(ssh ubuntu@vivid-slave.vapour.ws lsb_release -sr)
SERIES=$(ssh ubuntu@vivid-slave.vapour.ws lsb_release -sc)
ARCH=$(ssh ubuntu@vivid-slave.vapour.ws dpkg --print-architecture)

$SCRIPTS/jujuci.py -v setup-workspace $WORKSPACE
# We need a way to lookup the build-source-packages number from the br number.
$SCRIPTS/jujuci.py get -b lastBuild build-source-packages "*$RELEASE*"\
  $WORKSPACE
$SCRIPTS/jujuci.py get -b lastBuild build-source-packages '*orig.tar.gz'\
  $WORKSPACE

export RELEASE SERIES ARCH
python - *$RELEASE* *.orig.tar.gz <<"EOT"
import json
import os
from os.path import join
import sys
import subprocess
from tempfile import NamedTemporaryFile

revision_build = os.environ['revision_build']
job_name = os.environ['JOB_NAME']
build_number = os.environ['BUILD_NUMBER']
prefix = 'juju-ci/products/version-{}/{}/build-{}'.format(
    revision_build, job_name, build_number)
s3_config = join(os.environ['HOME'], 'cloud-city/juju-qa.s3cfg')

(dsc_file,) = [x for x in sys.argv[1:] if x.endswith('.dsc')]
command = [
    'mv', 'packages/*', '.', ';', 'pwd', ';', 'ls', '-l', ';'
    '/home/ubuntu/juju-release-tools/build_package.py', '-v', 'binary',
    dsc_file, '$(pwd)', os.environ['SERIES'], os.environ['ARCH']]
with NamedTemporaryFile() as config_file:
    json.dump({
        'command': command,
        'install': {'packages': sys.argv[1:]},
        'artifacts': {'packages': ['*.deb']},
        'bucket': 'juju-qa-data',
        }, config_file)
    config_file.flush()
    subprocess.check_call([
        join(os.environ['HOME'], 'workspace-runner', 'workspace-run'),
        '-v',
        config_file.name,
        'ubuntu@vivid-slave.vapour.ws',
        prefix,
        '--s3-config', s3_config,
        '-i', join(os.environ['JUJU_HOME'], 'staging-juju-rsa'),
        ])
EOT
