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


GOLANG_VERSION = '1.6'
CROSSCOMPILE_SOURCE = (
    'https://raw.githubusercontent.com'
    '/davecheney/golang-crosscompile/master/crosscompile.bash')
INNO_SOURCE = 'http://www.jrsoftware.org/download.php/is-unicode.exe?site=1'
ISCC_CMD = os.path.expanduser(
    '~/.wine/drive_c/Program Files (x86)/Inno Setup 5/ISCC.exe')
JUJU_PACKAGE_PATH = os.path.join('github.com', 'juju', 'juju')
ISS_DIR = os.path.join('src', JUJU_PACKAGE_PATH, 'scripts', 'win-installer')


@contextmanager
def go_tarball(tarball_path):
    """Context manager for setting the GOPATH from a golang tarball."""
    base_dir = mkdtemp()
    try:
        try:
            with tarfile.open(name=tarball_path, mode='r:gz') as tar:
                tar.extractall(path=base_dir)
        except (tarfile.ReadError, IOError):
            error_message = "Not a tar.gz: %s" % tarball_path
            raise ValueError(error_message)
        tarball_dir_name = os.path.basename(
            tarball_path.replace('.tar.gz', ''))
        version = tarball_dir_name.split('_')[-1]
        gopath = os.path.join(base_dir, tarball_dir_name)
        yield gopath, version
    finally:
        shutil.rmtree(base_dir)


@contextmanager
def working_directory(path):
    """Set the working directory for a block of operations."""
    saved_path = os.getcwd()
    try:
        os.chdir(path)
        yield path
    finally:
        os.chdir(saved_path)


def run_command(command, env=None, dry_run=False, verbose=False):
    """Optionally execute a command and maybe print the output."""
    if verbose:
        print('Executing: %s' % command)
    if not dry_run:
        output = subprocess.check_output(
            command, env=env, stderr=subprocess.STDOUT)
        if verbose:
            print(output)


def go_build(package, goroot, gopath, goarch, goos,
             dry_run=False, verbose=False):
    """Build and install a go package."""
    env = dict(os.environ)
    env['GOROOT'] = goroot
    env['GOPATH'] = gopath
    env['GOARCH'] = goarch
    env['GOOS'] = goos
    command = ['go', 'install', package]
    run_command(command, env=env, dry_run=dry_run, verbose=verbose)


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
    """Build a juju win client and installer from a tarball."""
    cwd = os.getcwd()
    cli_package = os.path.join(JUJU_PACKAGE_PATH, 'cmd', 'juju')
    goroot = os.path.join(build_dir, 'golang-%s' % GOLANG_VERSION)
    with go_tarball(tarball_path) as (gopath, version):
        # This command always executes in a tmp dir, it does not make changes.
        go_build(
            cli_package, goroot, gopath, '386', 'windows',
            dry_run=False, verbose=verbose)
        built_cli_path = os.path.join(gopath, 'bin', 'windows_386', 'juju.exe')
        make_installer(
            built_cli_path, version, gopath, cwd,
            dry_run=dry_run, verbose=verbose)


def make_installer(built_cli_path, version, gopath, dest_dir,
                   dry_run=False, verbose=False):
    """Create an installer for the windows juju client."""
    iss_dir = os.path.join(gopath, ISS_DIR)
    shutil.move(built_cli_path, iss_dir)
    with working_directory(iss_dir):
        # This command always executes in a tmp dir, it does not make changes.
        iss_cmd = ['xvfb-run', 'wine', ISCC_CMD, 'setup.iss']
        run_command(iss_cmd, dry_run=False, verbose=verbose)
        installer_name = 'juju-setup-%s.exe' % version
        installer_path = os.path.join(iss_dir, 'Output', installer_name)
        if not dry_run:
            if verbose:
                print('Moving %s to %s' % (installer_path, dest_dir))
            shutil.move(installer_path, dest_dir)


def build_win_agent(tarball_path, build_dir, dry_run=False, verbose=False):
    """Build a windows juju agent from a tarball."""
    cwd = os.getcwd()
    agent_package = os.path.join(JUJU_PACKAGE_PATH, 'cmd', 'jujud')
    goroot = os.path.join(build_dir, 'golang-%s' % GOLANG_VERSION)
    with go_tarball(tarball_path) as (gopath, version):
        # This command always executes in a tmp dir, it does not make changes.
        go_build(
            agent_package, goroot, gopath, 'amd64', 'windows',
            dry_run=False, verbose=verbose)
        built_agent_path = os.path.join(
            gopath, 'bin', 'windows_amd64', 'jujud.exe')
        make_agent_tarball(
            'win2012', built_agent_path, version, cwd,
            dry_run=dry_run, verbose=verbose)


def make_agent_tarball(series, built_agent_path, version, dest_dir,
                       dry_run=False, verbose=False):
    """Create a agent tgz for a jujud."""
    agent_tarball_name = 'juju-%s-%s-amd64.tgz' % (version, series)
    agent_tarball_path = os.path.join(dest_dir, agent_tarball_name)
    if not dry_run:
        if verbose:
            print('Creating %s' % agent_tarball_path)
        with tarfile.open(name=agent_tarball_path, mode='w:gz') as tar:
            if verbose:
                print('Adding %s' % built_agent_path)
            arcname = os.path.basename(built_agent_path)
            tar.add(built_agent_path, arcname=arcname)


def build_osx_client(tarball_path, build_dir, dry_run=False, verbose=False):
    """Build an OS X client and plugins from a tarball."""
    cwd = os.getcwd()
    cmd_package = '%s/cmd/...' % JUJU_PACKAGE_PATH
    goroot = os.path.join(build_dir, 'golang-%s' % GOLANG_VERSION)
    with go_tarball(tarball_path) as (gopath, version):
        # This command always executes in a tmp dir, it does not make changes.
        go_build(
            cmd_package, goroot, gopath, 'amd64', 'darwin',
            dry_run=False, verbose=verbose)
        if sys.platform == 'darwin':
            bin_path = os.path.join(gopath, 'bin')
        else:
            bin_path = os.path.join(gopath, 'bin', 'darwin_amd64')
        binary_paths = [
            os.path.join(bin_path, 'juju'),
            os.path.join(bin_path, 'juju-metadata'),
            os.path.join(gopath, ISS_DIR, 'README.txt'),
            os.path.join(gopath, 'src', JUJU_PACKAGE_PATH, 'LICENCE'),
        ]
        make_client_tarball(
            'osx', binary_paths, version, cwd,
            dry_run=dry_run, verbose=verbose)


def make_client_tarball(os_name, binary_paths, version, dest_dir,
                        dry_run=False, verbose=False):
    """Create a tarball of the built binaries and files."""
    os_tarball_name = 'juju-%s-%s.tar.gz' % (version, os_name)
    os_tarball_path = os.path.join(dest_dir, os_tarball_name)
    if not dry_run:
        if verbose:
            print('Creating %s' % os_tarball_path)
        with tarfile.open(name=os_tarball_path, mode='w:gz') as tar:
            ti = tarfile.TarInfo('juju-bin')
            ti.type = tarfile.DIRTYPE
            ti.mode = 0o775
            tar.addfile(ti)
            for binary_path in binary_paths:
                if verbose:
                    print('Adding %s' % binary_path)
                arcname = 'juju-bin/%s' % os.path.basename(binary_path)
                tar.add(binary_path, arcname=arcname)


def build_centos(tarball_path, build_dir, dry_run=False, verbose=False):
    """Build an Centos client, plugins and agent from a tarball."""
    cwd = os.getcwd()
    cmd_package = '%s/cmd/...' % JUJU_PACKAGE_PATH
    goroot = os.path.join(build_dir, 'golang-%s' % GOLANG_VERSION)
    with go_tarball(tarball_path) as (gopath, version):
        # This command always executes in a tmp dir, it does not make changes.
        go_build(
            cmd_package, goroot, gopath, 'amd64', 'linux',
            dry_run=False, verbose=verbose)
        bin_path = os.path.join(gopath, 'bin')
        built_agent_path = os.path.join(bin_path, 'jujud')
        binary_paths = [
            os.path.join(bin_path, 'juju'),
            built_agent_path,
            os.path.join(bin_path, 'juju-metadata'),
            os.path.join(gopath, ISS_DIR, 'README.txt'),
            os.path.join(gopath, 'src', JUJU_PACKAGE_PATH, 'LICENCE'),
        ]
        make_client_tarball(
            'centos7', binary_paths, version, cwd,
            dry_run=dry_run, verbose=verbose)
        make_agent_tarball(
            'centos7', built_agent_path, version, cwd,
            dry_run=dry_run, verbose=verbose)


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
        help='Build a 386 windows juju client and an installer.')
    parser_win_client.add_argument(
        '-b', '--build-dir', default='$HOME/crossbuild',
        help='The path cross build dir.')
    parser_win_client.add_argument(
        'tarball_path', help='The path to the juju source tarball.')
    # ./crossbuild win-agent juju-core-1.2.3.tar.gz
    parser_win_agent = subparsers.add_parser(
        'win-agent', help='Build an amd64 windows juju agent.')
    parser_win_agent.add_argument(
        '-b', '--build-dir', default='~/crossbuild',
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
    # ./crossbuild centos juju-core-1.2.3.tar.gz
    parser_centos = subparsers.add_parser(
        'centos', help='Build an amd64 Centos client, plugins, and agent.')
    parser_centos.add_argument(
        '-b', '--build-dir', default='$HOME/crossbuild',
        help='The path cross build dir.')
    parser_centos.add_argument(
        'tarball_path', help='The path to the juju source tarball.')
    return parser.parse_args(args)


def main(argv):
    """Cross build juju for an OS, arch, and client or server."""
    args = parse_args(argv)
    args.build_dir = os.path.expanduser(args.build_dir)
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
        elif args.command == 'centos':
            build_centos(
                args.tarball_path, args.build_dir,
                dry_run=args.dry_run, verbose=args.verbose)
    except Exception as e:
        print(e)
        print(getattr(e, 'output', ''))
        traceback.print_tb(sys.exc_info()[2])
        return 2
    if args.verbose:
        print("Done.")
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv[1:]))
