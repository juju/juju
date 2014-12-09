#!/usr/bin/python
"""Build juju for windows and darwin on 386 and amd64."""

from __future__ import print_function

from argparse import ArgumentParser
import os
import shutil
import subprocess
import sys
import traceback

GOLANG_VERSION = '1.2.1'
CROSSCOMPILE_SOURCE = (
    'https://raw.githubusercontent.com'
    '/davecheney/golang-crosscompile/master/crosscompile.bash')
INNO_SOURCE = 'http://www.jrsoftware.org/download.php/is-unicode.exe?site=1'


def run_command(command, dry_run=False, verbose=False, **kwargs):
    """Optionally xecute a command and maybe print the output."""
    if verbose:
        print('Executing: %s' % command)
    if not dry_run:
        kwargs['stderr'] = subprocess.STDOUT
        output = subprocess.check_output(command)
        if verbose:
            print(output)


def setup_cross_building(build_dir, dry_run=False, verbose=False):
    """Setup a cross building directory.

    This is not implemented but this was manually done following these steps:

    mkdir crossbuild
    cd crossbuild
    sudo apt-get install dpkg-dev wine xvfb
    apt-get source golang-go={GOLANG_VERSION}*
    export GOROOT=/var/lib/jenkins/crossbuild/golang-{GOLANG_VERSION}

    wget {CROSSCOMPILE_SOURCE} -O crosscompile.bash
    source crosscompile.bash
    go-crosscompile-build darwin/amd64
    go-crosscompile-build windows/386
    go-crosscompile-build windows/amd64

    wget {INNO_SOURCE} -O isetup-5.5.5-unicode.exe
    xvfb-run wine isetup-5.5.5-unicode.exe /verysilent
    """
    print(setup_cross_building.__doc__.format(
        GOLANG_VERSION=GOLANG_VERSION, CROSSCOMPILE_SOURCE=CROSSCOMPILE_SOURCE,
        INNO_SOURCE=INNO_SOURCE))


def build_win_client(tarball_path, build_dir, dry_run=False, verbose=False):
    pass


def build_win_agent(tarball_path, build_dir, dry_run=False, verbose=False):
    pass


def build_osx_client(tarball_path, build_dir, dry_run=False, verbose=False):
    pass


def parse_args(args=None):
    """Return the argument parser for this program."""
    parser = ArgumentParser(
        "Build juju for windows and darwin on 386 and amd64.")
    parser.add_argument(
        '-d', '--dry-run', action='store_true', default=False,
        help='Do not make changes.')
    parser.add_argument(
        '-v', '--verbose', action='store_true', default=False,
        help='Increase verbosity.')
    subparsers = parser.add_subparsers(help='sub-command help', dest="command")
    # ./crossbuild setup
    parser_setup = subparsers.add_parser(
        'setup', help='Setup a cross-compiling environment')
    parser_setup.add_argument(
        '-b', '--build-dir', default='$HOME/crossbuild',
        help='The path cross build dir.')
    # ./crossbuild win-client juju-core-1.2.3.tar.gz
    parser_win_client = subparsers.add_parser(
        'win-client',
        help='Build a 386 windown juju client and an installer.')
    parser_win_client.add_argument(
        '-b', '--build-dir', default='$HOME/crossbuild',
        help='The path cross build dir.')
    parser_win_client.add_argument(
        'tarball_path', help='The path to the juju source tarball.')
    # ./crossbuild win-agent juju-core-1.2.3.tar.gz
    parser_win_agent = subparsers.add_parser(
        'win-agent', help='Build an amd64 windows juju agent.')
    parser_win_agent.add_argument(
        '-b', '--build-dir', default='$HOME/crossbuild',
        help='The path cross build dir.')
    parser_win_agent.add_argument(
        'tarball_path', help='The path to the juju source tarball.')
    # ./crossbuild osx-client juju-core-1.2.3.tar.gz
    parser_osx_client = subparsers.add_parser(
        'osx-client', help='Build an amd64 OS X client and plugins.')
    parser_osx_client.add_argument(
        '-b', '--build-dir', default='$HOME/crossbuild',
        help='The path cross build dir.')
    parser_osx_client.add_argument(
        'tarball_path', help='The path to the juju source tarball.')
    return parser.parse_args(args)


def main(argv):
    """Cross build juju for an OS, arch, and client or server."""
    args = parse_args(argv)
    try:
        if args.command == 'setup':
            setup_cross_building(
                args.build_dir, dry_run=args.dry_run, verbose=args.verbose)
        elif args.command == 'win-client':
            build_win_client(
                args.tarball_path, args.build_dir,
                dry_run=args.dry_run, verbose=args.verbose)
        elif args.command == 'win-agent':
            build_win_agent(
                args.tarball_path, args.build_dir,
                dry_run=args.dry_run, verbose=args.verbose)
        elif args.command == 'osx-client':
            build_osx_client(
                args.tarball_path, args.build_dir,
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
