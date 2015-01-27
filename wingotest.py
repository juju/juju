from __future__ import print_function

from argparse import ArgumentParser
import shutil
import os
import subprocess
import sys
import tarfile
import traceback


GO_CMD = os.path.join('\\', 'go', 'bin', 'go.exe')

CI_DIR = os.path.abspath(os.path.join('\\', 'Users', 'Administrator', 'ci'))
TMP_DIR = os.path.abspath(os.path.join(CI_DIR, 'tmp'))
GOPATH = os.path.join(CI_DIR, 'gogo')


class WorkingDirectory:
    """Context manager for changing the current working directory"""
    def __init__(self, working_path):
        self.working_path = working_path

    def __enter__(self):
        self.savedPath = os.getcwd()
        os.chdir(self.working_path)

    def __exit__(self, etype, value, traceback):
        os.chdir(self.savedPath)


def run(*command, **kwargs):
    """Run a command and return the stdout and stderr output."""
    kwargs['stderr'] = subprocess.STDOUT
    output = subprocess.check_output(command, **kwargs)
    return output


def setup(tarfile_name):
    """Setup the workspace; remove data from previous runs."""
    juju_tars = [
        n for n in os.listdir(CI_DIR) if 'tar.gz' in n and n != tarfile_name]
    for name in juju_tars:
        path = os.path.join(CI_DIR, name)
        os.remove(path)
        print('Removed {0}'.format(path))
    if os.path.exists(GOPATH):
        shutil.rmtree(GOPATH)
        print('Removed {0}'.format(GOPATH))
    if os.path.exists(TMP_DIR):
        shutil.rmtree(TMP_DIR)
    os.mkdir(TMP_DIR)


def untar(tarfile_path):
    """Untar the tarfile to the workspace temp dir."""
    error_message = None
    try:
        with tarfile.open(name=tarfile_path, mode='r:gz') as tar:
            tar.extractall(path=TMP_DIR)
    except tarfile.ReadError:
        error_message = "Not a tar.gz: %s" % tarfile_path
        raise Exception(error_message)
    print('Extracted the Juju source.')


def move_source_to_gopath(tarfile_name):
    """Move the extracted tarfile to the GOPATH."""
    dir_name = tarfile_name.replace('.tar.gz', '')
    dir_path = os.path.join(TMP_DIR, dir_name)
    shutil.move(dir_path, GOPATH)
    print('Moved {0} to {1}'.format(dir_path, GOPATH))


def go_test_package(package, go_cmd, gopath):
    """Run the package unit tests."""
    env = dict(os.environ)
    env['GOPATH'] = gopath
    env['GOARCH'] = 'amd64'
    package_dir = os.path.join(gopath, 'src', package.replace('/', os.sep))
    with WorkingDirectory(package_dir):
        output = run(go_cmd, 'test', './...', env=env)
        print(output)
        print('Completed unit tests')


def parse_args(args=None):
    """Return the argument parser for this program."""
    parser = ArgumentParser("Run go test against the content of a tarfile.")
    parser.add_argument(
        '-d', '--dry-run', action='store_true', default=False,
        help='Do not make changes.')
    parser.add_argument(
        '-v', '--verbose', action='store_true', default=False,
        help='Increase verbosity.')
    parser.add_argument(
        '-p', 'package', default='github/juju/juju',
        help='The package to test.')
    parser.add_argument(
        'tarfile', help='The path to the gopath tarfile.')
    return parser.parse_args(args)


def main(argv):
    """Run go test against the content of a tarfile."""
    args = parse_args(argv)
    # ssh-keygen -t rsa -b 2048 -N "" -f ~/.ssh/id_rsa
    tarfile_name = args.tarfile
    version = tarfile_name.split('_')[-1].replace('.tar.gz', '')
    tarfile_path = os.path.abspath(os.path.join(CI_DIR, tarfile_name))
    try:
        print('Testing juju {0} from {1}'.format(
            version, tarfile_name))
        setup(tarfile_name)
        untar(tarfile_path)
        move_source_to_gopath(tarfile_name)
        go_test_package(args.package, GO_CMD, GOPATH)
    except Exception as e:
        print(str(e))
        if isinstance(e, subprocess.CalledProcessError):
            print("COMMAND OUTPUT:")
            print(e.output)
        print(traceback.print_tb(sys.exc_info()[2]))
        return 3
    return 0


if __name__ == '__main__':
    sys.exit(main())
