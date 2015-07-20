#!/usr/bin/env python
from glob import glob
import json
import os
from os.path import join
import sys
import subprocess
from tempfile import NamedTemporaryFile


def main():
    os.environ['PATH'] = '{}:{}'.format(
        os.environ['SCRIPTS'], os.environ['PATH'])
    revision_build = os.environ['revision_build']
    job_name = os.environ['JOB_NAME']
    build_number = os.environ['BUILD_NUMBER']
    workspace = os.environ['WORKSPACE']
    host = sys.argv[1]
    s3_config = join(os.environ['HOME'], 'cloud-city/juju-qa.s3cfg')

    subprocess.check_call(['jujuci.py', '-v', 'setup-workspace', workspace])

    arch = subprocess.check_output(['ssh', host, 'dpkg',
                                    '--print-architecture']).rstrip()
    series = subprocess.check_output(
        ['ssh', host, 'lsb_release', '-sc']).rstrip()
    release = subprocess.check_output(
        ['ssh', host, 'lsb_release', '-sr']).rstrip()
    release_glob = '*{}*'.format(release)

    subprocess.check_call([
        'jujuci.py', 'get', '-b', 'lastBuild', 'build-source-packages',
        release_glob, workspace])
    packages = glob(release_glob)
    (dsc_file,) = [x for x in packages if x.endswith('.dsc')]
    subprocess.check_call(
        ['jujuci.py', 'get', '-b', 'lastBuild', 'build-source-packages',
         '*orig.tar.gz', workspace])
    packages.extend(glob('*.orig.tar.gz'))
    command = [
        'mv', 'packages/*', '.', ';', 'pwd', ';', 'ls', '-l', ';'
        '/home/ubuntu/juju-release-tools/build_package.py', '-v', 'binary',
        dsc_file, '$(pwd)', series, arch]
    prefix = 'juju-ci/products/version-{}/{}/build-{}'.format(
        revision_build, job_name, build_number)
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
            host,
            prefix,
            '--s3-config', s3_config,
            '-i', join(os.environ['JUJU_HOME'], 'staging-juju-rsa'),
            ])


if __name__ == '__main__':
    main()
