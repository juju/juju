#!/bin/bash
set -eux
host=$1
RELEASE=$(ssh $host lsb_release -sr)
SERIES=$(ssh $host lsb_release -sc)
ARCH=$(ssh $host dpkg --print-architecture)
PATH=$SCRIPTS:$PATH

# We need a way to lookup the build-source-packages number from the br number.

export RELEASE SERIES ARCH host PATH
python - <<"EOT"
from glob import glob
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
release = os.environ['RELEASE']

subprocess.check_call(['jujuci.py', '-v', 'setup-workspace', os.environ['WORKSPACE']])
release_glob = '*{}*'.format(release)
subprocess.check_call([
  'jujuci.py', 'get', '-b', 'lastBuild', 'build-source-packages', release_glob,
  os.environ['WORKSPACE']])
packages = glob(release_glob)
(dsc_file,) = [x for x in packages if x.endswith('.dsc')]
subprocess.check_call(
  ['jujuci.py', 'get', '-b', 'lastBuild', 'build-source-packages',
   '*orig.tar.gz', os.environ['WORKSPACE']])
packages.extend(glob('*.orig.tar.gz'))
command = [
    'mv', 'packages/*', '.', ';', 'pwd', ';', 'ls', '-l', ';'
    '/home/ubuntu/juju-release-tools/build_package.py', '-v', 'binary',
    dsc_file, '$(pwd)', os.environ['SERIES'], os.environ['ARCH']]
with NamedTemporaryFile() as config_file:
    json.dump({
        'command': command,
        'install': {'packages': packages},
        'artifacts': {'packages': ['*.deb']},
        'bucket': 'juju-qa-data',
        }, config_file)
    config_file.flush()
    subprocess.check_call([
        join(os.environ['HOME'], 'workspace-runner', 'workspace-run'),
        '-v',
        config_file.name,
        os.environ['host'],
        prefix,
        '--s3-config', s3_config,
        '-i', join(os.environ['JUJU_HOME'], 'staging-juju-rsa'),
        ])
EOT
