#!/usr/bin/python
"""Build and package juju for an non-debian OS."""

from __future__ import print_function

from argparse import ArgumentParser
import sys
import traceback

from candidate import run_command
from jujuci import (
    get_artifacts,
    BUILD_REVISION,
    setup_workspace,
)


def build_juju(product, workspace_dir, build, dry_run=False, verbose=False):
    #./crossbuild.py -v win-client -b $HOME/crossbuild juju-core_1.20.12.tar.gz
    # juju-setup-1.20.12.exe
    #./crossbuild.py -v win-agent -b $HOME/crossbuild juju-core_1.20.12.tar.gz
    # juju-1.20.12-win2012-amd64.tgz
    #./crossbuild.py -v osx-client -b $HOME/crossbuild juju-core_1.20.12.tar.gz
    # juju-1.20.12-osx.tar.gz
    setup_workspace(workspace_dir, dry_run=dry_run, verbose=verbose)
    artifacts = get_artifacts(
        BUILD_REVISION, build, 'juju-core-*.tar.gz', workspace_dir,
        archive=False, dry_run=dry_run, verbose=verbose)
    tar_artifact = artifacts[0]
    command = [
        'crossbuild.py', product, '-b', '~/crossbuild', tar_artifact.file_name]
    run_command(command, dry_run=dry_run, verbose=verbose)


def parse_args(args=None):
    """Return the argument parser for this program."""
    parser = ArgumentParser("Build and package juju for an non-debian OS.")
    parser.add_argument(
        '-d', '--dry-run', action='store_true', default=False,
        help='Do not make changes.')
    parser.add_argument(
        '-v', '--verbose', action='store_true', default=False,
        help='Increase verbosity.')
    parser.add_argument(
        '-b', '--build', default='lastSuccessfulBuild',
        help="The specific revision-build number to get the tarball from")
    parser.add_argument(
        'product', choices=['win-client', 'win-agent', 'osx-agent'],
        help='the kind of juju to make and package.')
    parser.add_argument(
        'workspace',  help='The path to the workspace to build in.')
    return parser.parse_args(args)


def main(argv):
    """Manage list and get files from jujuci builds."""
    args = parse_args(argv)
    try:
        build_juju(
            args.product, args.build, args.workspace,
            dry_run=args.dry_run, verbose=args.verbose)
    except Exception as e:
        print(e)
        if args.verbose:
            traceback.print_tb(sys.exc_info()[2])
        return 2
    if args.verbose:
        print("Done.")
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv[1:]))
