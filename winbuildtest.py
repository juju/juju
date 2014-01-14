#!/usr/bin/python

# This script builds the windows juju installer and verifies it works.
# This is run on a window's machine with sshd, python and go installed.
# CI must send this script and the tarball under test to the windows
# machine, then exec via ssh this.

from __future__ import print_function

import shutil
import os
import subprocess
import sys
import tarfile


GO_CMD = os.path.join('\\', 'go', 'bin', 'go.exe')
ISS_CMD = os.path.join('\\', 'Progra~2', 'InnoSe~1', 'iscc.exe')
JUJU_CMD = os.path.join('\\', 'Progra~2', 'Juju', 'juju.exe')
JUJU_UNINSTALL = os.path.join('\\', 'Progra~2', 'Juju', 'uninstall.exe')

CI_DIR = os.path.abspath(os.path.join('\\', 'Users', 'Administrator', 'ci'))
TMP_DIR = os.path.abspath(os.path.join(CI_DIR, 'tmp'))
GOPATH = os.path.join(CI_DIR, 'gogo')
JUJU_CMD_DIR = os.path.join(
    GOPATH, 'src', 'launchpad.net', 'juju-core', 'cmd', 'juju')
ISS_DIR = os.path.join(
    GOPATH, 'src', 'launchpad.net', 'juju-core', 'scripts', 'win-installer')


class WorkingDirectory:
    """Context manager for changing the current working directory"""
    def __init__(self, working_path):
        self.working_path = working_path

    def __enter__(self):
        self.savedPath = os.getcwd()
        os.chdir(self.working_path)

    def __exit__(self, etype, value, traceback):
        os.chdir(self.savedPath)


def run(command, *args, **kwargs):
    try:
        output = subprocess.check_output(command, *args, **kwargs)
        return output
    except Exception as e:
        print(str(e))
        raise


def is_sane(tarball_path):
    if os.path.exists(tarball_path) and tarball_path.startswith(CI_DIR):
        return True
    else:
        print(
            "Tarball not found: {0}".format(tarball_path))
        return False


def setup(tarball_name):
    juju_tars = [
        n for n in os.listdir(CI_DIR) if 'tar.gz' in n and n != tarball_name]
    for name in juju_tars:
        path = os.path.join(CI_DIR, name)
        os.remove(path)
        print('Removed {0}'.format(path))
    juju_execs = [
        n for n in os.listdir(CI_DIR) if 'juju-setup' in n and '.exe' in n]
    for name in juju_execs:
        path = os.path.join(CI_DIR, name)
        os.remove(path)
        print('Removed {0}'.format(path))
    if os.path.exists(GOPATH):
        shutil.rmtree(GOPATH)
        print('Removed {0}'.format(GOPATH))
    if os.path.exists(JUJU_UNINSTALL):
        run(JUJU_UNINSTALL, '/verysilent')
        print('Uninstalled Juju with {0}'.format(JUJU_UNINSTALL))
    if os.path.exists(TMP_DIR):
        shutil.rmtree(TMP_DIR)
    os.mkdir(TMP_DIR)


def untar(tarball_path):
    error_message = None
    try:
        with tarfile.open(name=tarball_path, mode='r:gz') as tar:
            tar.extractall(path=TMP_DIR)
    except tarfile.ReadError:
        error_message = "Not a tar.gz: %s" % tarball_path
        raise Exception(error_message)
    print('Extracted the Juju source.')


def move_source_to_gopath(tarball_name):
    dir_name = tarball_name.replace('.tar.gz', '')
    dir_path = os.path.join(TMP_DIR, dir_name)
    shutil.move(dir_path, GOPATH)
    print('Moved {0} to {1}'.format(dir_path, GOPATH))


def build():
    env = dict(os.environ)
    env['GOPATH'] = GOPATH
    env['GOARCH'] = '386'
    with WorkingDirectory(JUJU_CMD_DIR):
        output = run([GO_CMD, 'build'], env=env)
        print(output)
        print('Built Juju.exe')
        shutil.move('juju.exe', ISS_DIR)
        print('Moved {0} to {1}'.format('juju.exe', ISS_DIR))


def package(version):
    with WorkingDirectory(ISS_DIR):
        output = run([ISS_CMD, 'setup.iss'])
        print(output)
        installer_name = 'juju-setup-{0}.exe'.format(version)
        installer_path = os.path.join(ISS_DIR, 'output', installer_name)
        shutil.move(installer_path, CI_DIR)
        print('Moved {0} to {1}'.format(installer_path, CI_DIR))
    return installer_name


def install(installer_name):
    installer_path = os.path.join(CI_DIR, installer_name)
    output = run([installer_path, '/verysilent'])
    print(output)
    print('Installed Juju')


def test(version):
    output = run([JUJU_CMD, 'version'])
    print(output)
    if version not in output:
        raise Exception("Juju did not install")


def main():
    if len(sys.argv) != 2:
        print('USAGE: {0} juju-core_X.X.X.tar.gz')
        return 1
    tarball_name = sys.argv[1]
    version = tarball_name.split('_')[-1].replace('.tar.gz', '')
    tarball_path = os.path.abspath(os.path.join(CI_DIR, tarball_name))
    if not is_sane(tarball_path):
        return 2
    try:
        print('Building and installing Juju {0} from {1}'.format(
            version, tarball_name))
        setup(tarball_name)
        untar(tarball_path)
        move_source_to_gopath(tarball_name)
        build()
        installer_name = package(version)
        install(installer_name)
        test(version)
        return 0
    except Exception as e:
        print(str(e))
        print(sys.exc_info()[0])
        return 3
    return 0


if __name__ == '__main__':
    sys.exit(main())
