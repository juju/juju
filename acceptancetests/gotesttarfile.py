#!/usr/bin/python
from __future__ import print_function

from argparse import ArgumentParser
import shutil
import os
import subprocess
import sys
import tarfile
import tempfile
import traceback

from utility import (
    print_now,
    temp_dir,
)


class WorkingDirectory:
    """Context manager for changing the current working directory"""
    def __init__(self, working_path):
        self.working_path = working_path

    def __enter__(self):
        self.savedPath = os.getcwd()
        os.chdir(self.working_path)

    def __exit__(self, etype, value, traceback):
        os.chdir(self.savedPath)


def run(command, **kwargs):
    """Run a command returning exit status."""
    proc = subprocess.Popen(command, **kwargs)
    proc.communicate()
    return proc.returncode


def untar_gopath(tarfile_path, gopath, delete=False, verbose=False):
    """Untar the tarfile to the gopath."""
    with temp_dir() as tmp_dir:
        with tarfile.open(name=tarfile_path, mode='r:gz') as tar:
            tar.extractall(path=tmp_dir)
        if verbose:
            print_now('Extracted the Juju source.')
        dir_name = os.path.basename(tarfile_path).replace('.tar.gz', '')
        dir_path = os.path.join(tmp_dir, dir_name)
        shutil.move(dir_path, gopath)
        if verbose:
            print_now('Moved %s to %s' % (dir_name, gopath))
    if delete:
        os.unlink(tarfile_path)
        if verbose:
            print_now('Deleted %s' % tarfile_path)


def murder_mongo():
    """Kill off any lingering mongod processess."""
    if sys.platform == 'win32':
        return run(["taskkill.exe", "/F", "/FI", "imagename eq mongod.exe"])
    return run(["sudo", "killall", "-SIGABRT", "mongod"])


def go_test_package(package, go_cmd, gopath, verbose=False):
    """Run the package unit tests."""
    # Set GOPATH and GOARCH to ensure the go command tests extracted
    # tarfile using the arch the win-agent is compiled with. The
    # default go env might be 386 used to create a win client.
    env = dict(os.environ)
    env['GOPATH'] = gopath
    env['GOARCH'] = 'amd64'
    version_cmd = [go_cmd, 'version']
    mongo_version_cmd = ['mongod', '--version']
    env_cmd = [go_cmd, 'env']
    build_cmd = [go_cmd, 'test', '-i', './...']
    test_cmd = [go_cmd, 'test', '-timeout=1200s', './...']
    if sys.platform == 'win32':
        # Ensure OpenSSH is never in the path for win tests.
        sane_path = [p for p in env['PATH'].split(';') if 'OpenSSH' not in p]
        env['PATH'] = ';'.join(sane_path)
        if verbose:
            print_now('Setting environ Path to:')
            print_now(env['PATH'])
        # GZ 2015-04-21: Short-term hack to work around case-insensitive issues
        env['Path'] = env.pop('PATH')
        tempdir = tempfile.mkdtemp(prefix="tmp-juju-test", dir=gopath)
        env['TMP'] = env['TEMP'] = tempdir
        if verbose:
            print_now('Setting environ TMP and TEMP to:')
            print_now(env['TEMP'])
        build_cmd = ['powershell.exe', '-Command'] + build_cmd
        test_cmd = ['powershell.exe', '-Command'] + test_cmd
        version_cmd = ['powershell.exe', '-Command'] + version_cmd
        mongo_version_cmd = ['powershell.exe', '-Command'] + mongo_version_cmd
        env_cmd = ['powershell.exe', '-Command'] + env_cmd
    package_dir = os.path.join(gopath, 'src', package.replace('/', os.sep))
    with WorkingDirectory(package_dir):
        if verbose:
            print_now('Go environment information')
        # We don't care about return code for these
        run(version_cmd, env=env)
        run(env_cmd, env=env)
        if verbose:
            print_now('Mongo environment information')
        run(mongo_version_cmd, env=env)
        if verbose:
            print_now('Building test dependencies')
        returncode = run(build_cmd, env=env)
        if returncode != 0:
            return returncode
        if verbose:
            print_now('Running unit tests in %s' % package)
        returncode = run(test_cmd, env=env)
        if verbose:
            if returncode == 0:
                print_now('SUCCESS')
            else:
                print_now('FAIL')
            print_now("Killing any lingering mongo processes...")
        murder_mongo()
    return returncode


# suppress nosetests
go_test_package.__test__ = False


def parse_args(args=None):
    """Return parsed args for this program."""
    parser = ArgumentParser("Run go test against the content of a tarfile.")
    parser.add_argument(
        '-v', '--verbose', action='store_true', default=False,
        help='Increase verbosity.')
    parser.add_argument(
        '-g', '--go', default='go', help='The go comand.')
    parser.add_argument(
        '-p', '--package', default='github.com/juju/juju',
        help='The package to test.')
    parser.add_argument(
        '-r', '--remove-tarfile', action='store_true', default=False,
        help='Remove the tarfile after extraction.')
    parser.add_argument(
        'tarfile', help='The path to the gopath tarfile.')
    return parser.parse_args(args)


def main(argv=None):
    """Run go test against the content of a tarfile."""
    returncode = 0
    args = parse_args(argv)
    tarfile_path = args.tarfile
    tarfile_name = os.path.basename(tarfile_path)
    version = tarfile_name.split('_')[-1].replace('.tar.gz', '')
    try:
        if args.verbose:
            print_now('Testing juju %s from %s' % (version, tarfile_name))
        with temp_dir() as workspace:
            gopath = os.path.join(workspace, 'gogo')
            untar_gopath(
                tarfile_path, gopath, delete=args.remove_tarfile,
                verbose=args.verbose)
            returncode = go_test_package(
                args.package, args.go, gopath, verbose=args.verbose)
    except Exception as e:
        print_now(str(e))
        print_now(traceback.print_exc())
        return 3
    return returncode


if __name__ == '__main__':
    sys.exit(main())
