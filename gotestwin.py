#!/usr/bin/env python
from argparse import ArgumentParser
from json import dump
import os
from os.path import (
    basename,
    dirname,
    join,
)
import subprocess
import sys


SCRIPTS = dirname(__file__)


def main(argv=None):
    parser = ArgumentParser()
    parser.add_argument('host', help='The machine to test on.')
    parser.add_argument('revision_or_tarfile',
                        help='The revision-build or tarfile path to test.')
    parser.add_argument('package', nargs='?', default='github.com/juju/juju',
                        help='The package to test.')
    args = parser.parse_args(argv)

    if args.revision_or_tarfile.endswith('tar.gz'):
        downloaded = args.revision_or_tarfile
    else:
        revision = args.revision_or_tarfile
        s3_ci_path = join(SCRIPTS, 's3ci.py')
        downloaded = subprocess.check_output([
            s3_ci_path, 'get', revision, 'build-revision',
            '.*.tar.gz', './'])
        job_name = os.environ.get('job_name', 'GoTestWin')
        subprocess.check_call([s3_ci_path, 'get-summary', revision, job_name])
    tarfile = basename(downloaded)
    with open('temp-config.yaml', 'w') as temp_file:
        dump({
            'install': {'ci': [
                tarfile,
                join(SCRIPTS, 'gotesttarfile.py'),
                join(SCRIPTS, 'jujucharm.py'),
                join(SCRIPTS, 'utility.py'),
                ]},
            'command': [
                'python', 'ci/gotesttarfile.py', '-v', '-g', 'go.exe', '-p',
                args.package, '--remove', 'ci/{}'.format(tarfile)
                ]},
             temp_file)
    juju_home = os.environ.get('JUJU_HOME',
                               join(dirname(SCRIPTS), 'cloud-city'))
    subprocess.check_call([
        'workspace-run', '-v', '-i', join(juju_home, 'staging-juju-rsa'),
        'temp-config.yaml', 'Administrator@{}'.format(args.host)
        ])


if __name__ == '__main__':
    main(sys.argv[1:])
