#!/usr/bin/python
from argparse import ArgumentParser
import os
from tempfile import NamedTemporaryFile
import yaml

from utility import run_command

from schedule_hetero_control import get_candidate_info
from utility import (
    ensure_dir,
    find_candidates,
    get_candidates_path,
)


def get_revisions(root_dir):
    revisions = []
    ensure_dir(get_candidates_path(root_dir))
    for candidate in find_candidates(root_dir):
        _, rev = get_candidate_info(candidate)
        revisions.append(rev)
    return revisions


def create_workspace_yaml(juju_home, script_path, stream, rev=None):
    rev = '-c {}'.format(" ".join(rev)) if rev else ''
    yl = {
        "install": {
            "cloud-city": [os.path.join(juju_home, 'ec2rc')]},
        "command": [
            "python",
            script_path,
            "cloud-city", "-r", "-v", rev]
    }
    yaml.safe_dump(yl, stream)


def parse_args(args=None):
    parser = ArgumentParser(
        "Run download_juju script on the OS X and Windows machines.")
    parser.add_argument('-o', '--osx-host',
                        default='jenkins@osx-slave.vapour.ws',
                        help="OS X machine's username and hostname.")
    parser.add_argument('-w', '--win-host',
                        default='Administrator@win-slave.vapour.ws',
                        help="Windows' username and hostname.")
    parser.add_argument('-j', '--juju-home',
                        default=os.environ.get('JUJU_HOME'),
                        help="Juju home directory (cloud-city dir).")
    parsed_args = parser.parse_args(args)
    if parsed_args.juju_home is None:
        parser.error(
            'Invalid JUJU_HOME value: either set $JUJU_HOME env variable or '
            'use --juju-home option to set the value.')

    return parsed_args


def main(argv=None):
    args = parse_args(argv)
    juju_home = args.juju_home
    win_host = args.win_host
    win_path = 'C:\\Users\\Administrator\\juju-ci-tools\\download_juju.py'
    osx_path = '$HOME/juju-ci-tools/download_juju.py'
    osx_host = args.osx_host
    rev = get_revisions(os.environ['HOME'])
    with NamedTemporaryFile() as temp_file:
        for path, host in [[win_path, win_host], [osx_path, osx_host]]:
            create_workspace_yaml(juju_home, path, temp_file, rev)
            run_command(['workspace-run', temp_file.name, host])


if __name__ == '__main__':
    main()
