#!/usr/bin/python
"""Build juju for windows and darwin on 386 and amd64."""

from __future__ import print_function

from argparse import ArgumentParser
from contextlib import contextmanager
import os
import shutil
import subprocess
import sys
import tarfile
from tempfile import mkdtemp
import traceback

GOLANG_VERSION = '1.2.1'
CROSSCOMPILE_SOURCE = (
    'https://raw.githubusercontent.com'
    '/davecheney/golang-crosscompile/master/crosscompile.bash')
INNO_SOURCE = 'http://www.jrsoftware.org/download.php/is-unicode.exe?site=1'


@contextmanager
def go_tarball(tarball_path):
    """Context manager for setting the GOPATH from a golang tarball."""
    try:
        base_dir = mkdtemp()
        try:
            with tarfile.open(name=tarball_path, mode='r:gz') as tar:
                tar.extractall(path=base_dir)
        except (tarfile.ReadError, IOError):
            error_message = "Not a tar.gz: %s" % tarball_path
            raise ValueError(error_message)
        tarball_dir_name = os.path.basename(
            tarball_path.replace('.tar.gz', ''))
        gopath = os.path.join(base_dir, tarball_dir_name)
        yield gopath
    finally:
        shutil.rmtree(base_dir)


@contextmanager
def working_directory(path):
    try:
        saved_path = os.getcwd()
        os.chdir(path)
        yield path
    finally:
        os.chdir(saved_path)


def run_command(command, env=None, dry_run=False, verbose=False):
    """Optionally xecute a command and maybe print the output."""
    if verbose:
        print('Executing: %s' % command)
    if not dry_run:
        output = subprocess.check_output(
            command, env=env, stderr=subprocess.STDOUT)
        if verbose:
            print(output)


def go_build(package, goroot, gopath, goarch, goos,
             dry_run=False, verbose=False):
    env = dict(os.environ)
    env['GOROOT'] = goroot
    env['GOPATH'] = gopath
    env['GOARCH'] = goarch
    env['GOOS'] = goos
    command = ['go', 'build', package]
    run_command(command, env=env, dry_run=dry_run, verbose=verbose)


def version_from_tarball(tarball_path):
    tarball_name = os.path.basename(tarball_path.replace('.tar.gz', ''))
    return tarball_name.split('_')[-1]


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
    cwd = os.getcwd()
    cli_package = os.path.join('github.com', 'juju', 'juju', 'cmd', 'juju')
    goroot = os.path.join(build_dir, 'golang-%s' % GOLANG_VERSION)
    with go_tarball(tarball_path) as gopath:
        go_build(
            cli_package, goroot, gopath, '386', 'windows',
            dry_run=dry_run, verbose=verbose)
        built_cli_path = os.path.join(gopath, 'src', cli_package, 'juju.exe')
        iss_dir = os.path.join(gopath, 'scripts', 'win-installer')
        version = version_from_tarball(tarball_path)
        create_installer(
            built_cli_path, version, iss_dir, cwd)


def create_installer(built_cli_path, version, iss_dir, dest_dir,
                     dry_run=False, verbose=False):
    iss_cmd = [
        'xvfb-run', 'wine',
        '~/.wine/drive_c/Program Files (x86)/Inno Setup 5/ISCC.exe',
        'setup.iss']
    shutil.move(built_cli_path, iss_dir)
    with working_directory(iss_dir):
        run_command(iss_cmd, dry_run=dry_run, verbose=verbose)
        installer_name = 'juju-setup-{0}.exe'.format(version)
        installer_path = os.path.join(iss_dir, 'output', installer_name)
        if not dry_run:
            shutil.move(installer_path, dest_dir)
            if verbose:
                print('Moved {0} to {1}'.format(installer_path, dest_dir))
    return installer_path


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
