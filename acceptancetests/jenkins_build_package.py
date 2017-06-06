#!/usr/bin/env python
from argparse import ArgumentParser
from glob import glob
import json
import os
from os.path import (
    dirname,
    join,
    )
import subprocess
from tempfile import NamedTemporaryFile


def main():
    scripts = dirname(__file__)
    parser = ArgumentParser()
    parser.add_argument('host', help='SSH user@hostname spec.')
    parser.add_argument('release', help='The distro version number.')
    parser.add_argument('series', help='The distro codename.')
    parser.add_argument('arch', help='The machine architecture.')
    parser.add_argument('source_package_build',
                        help='The build-source-package build to use.')
    args = parser.parse_args()
    os.environ['PATH'] = '{}:{}'.format(scripts, os.environ['PATH'])
    revision_build = os.environ['revision_build']
    job_name = os.environ['JOB_NAME']
    build_number = os.environ['BUILD_NUMBER']
    workspace = os.environ['WORKSPACE']
    s3_config = join(os.environ['JUJU_HOME'], 'juju-qa.s3cfg')

    subprocess.check_call(['jujuci.py', '-v', 'setup-workspace', workspace])

    release_glob = '.*{}.*'.format(args.release)

    subprocess.check_call([
        's3ci.py', 'get', revision_build,
        'build-source-packages', release_glob, workspace])
    packages = glob('*{}*'.format(args.release))
    (dsc_file,) = [x for x in packages if x.endswith('.dsc')]
    subprocess.check_call(
        ['s3ci.py', 'get', revision_build,
         'build-source-packages', '.*orig.tar.gz', workspace])
    packages.extend(glob('*.orig.tar.gz'))
    command = [
        'mv', 'packages/*', '.', ';',
        '/home/ubuntu/juju-release-tools/build_package.py', '-v', 'binary',
        dsc_file, '$(pwd)', args.series, args.arch]
    prefix = 'juju-ci/products/version-{}/{}/build-{}'.format(
        revision_build, job_name, build_number)
    private_key = join(os.environ['JUJU_HOME'], 'staging-juju-rsa')
    with NamedTemporaryFile() as config_file:
        json.dump({
            'command': command,
            'install': {'packages': packages},
            'artifacts': {'packages': ['*.deb']},
            'bucket': 'juju-qa-data',
            }, config_file)
        config_file.flush()
        subprocess.check_call([
            'workspace-run', '-v', config_file.name, args.host, prefix,
            '--s3-config', s3_config, '-i', private_key,
            ])


if __name__ == '__main__':
    main()
