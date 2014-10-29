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
import traceback


GO_CMD = os.path.join('\\', 'go', 'bin', 'go.exe')
ISS_CMD = os.path.join('\\', 'Progra~2', 'InnoSe~1', 'iscc.exe')
JUJU_CMD = os.path.join('\\', 'Progra~2', 'Juju', 'juju.exe')
JUJU_UNINSTALL = os.path.join('\\', 'Progra~2', 'Juju', 'unins000.exe')

GO_SRC_DIR = os.path.join('\\', 'go', 'src')
GCC_BIN_DIR = os.path.join('\\', 'MinGW', 'bin')
CI_DIR = os.path.abspath(os.path.join('\\', 'Users', 'Administrator', 'ci'))
TMP_DIR = os.path.abspath(os.path.join(CI_DIR, 'tmp'))
GOPATH = os.path.join(CI_DIR, 'gogo')
JUJU_CMD_DIR = os.path.join(
    GOPATH, 'src', 'github.com', 'juju', 'juju', 'cmd', 'juju')
JUJUD_CMD_DIR = os.path.join(
    GOPATH, 'src', 'github.com', 'juju', 'juju', 'cmd', 'jujud')
ISS_DIR = os.path.join(
    GOPATH, 'src', 'github.com', 'juju', 'juju', 'scripts', 'win-installer')


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
    kwargs['stderr'] = subprocess.STDOUT
    output = subprocess.check_output(command, **kwargs)
    return output


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
    agent_tars = [n for n in os.listdir(CI_DIR) if 'tgz' in n]
    for name in agent_tars:
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


def enable_cross_compile(gcc_bin_dir, go_src_dir, gopath):
    env = dict(os.environ)
    env['GOPATH'] = gopath
    env['PATH'] = '{}{}{}'.format(env['PATH'], os.pathsep, gcc_bin_dir)
    with WorkingDirectory(go_src_dir):
        for arch in ('amd64', '386'):
            env = dict(env)
            env['GOARCH'] = arch
            output = run('make.bat', '--no-clean', env=env)
            print(output)


def build_client(juju_cmd_dir, go_cmd, gopath, iss_dir):
    env = dict(os.environ)
    env['GOPATH'] = gopath
    env['GOARCH'] = '386'
    with WorkingDirectory(juju_cmd_dir):
        output = run(go_cmd, 'build', env=env)
        print(output)
        print('Built Juju.exe')
        shutil.move('juju.exe', iss_dir)
        print('Moved {0} to {1}'.format('juju.exe', iss_dir))


def create_installer(version, iss_dir, iss_cmd, ci_dir):
    with WorkingDirectory(iss_dir):
        output = run(iss_cmd, 'setup.iss')
        print(output)
        installer_name = 'juju-setup-{0}.exe'.format(version)
        installer_path = os.path.join(iss_dir, 'output', installer_name)
        shutil.move(installer_path, ci_dir)
        print('Moved {0} to {1}'.format(installer_path, ci_dir))
    return installer_name


def install(installer_name):
    installer_path = os.path.join(CI_DIR, installer_name)
    output = run(installer_path, '/verysilent')
    print(output)
    print('Installed Juju')


def test(version):
    output = run(JUJU_CMD, 'version')
    print(output)
    if version not in output:
        raise Exception("Juju did not install")


def has_agent(version):
    try:
        minor = int(version[2:4])
        return minor >= 21
    except ValueError:
        return False


def build_agent(jujud_cmd_dir, go_cmd, gopath):
    env = dict(os.environ)
    env['GOPATH'] = gopath
    env['GOARCH'] = 'amd64'
    with WorkingDirectory(jujud_cmd_dir):
        output = run(go_cmd, 'build', env=env)
        print(output)
        print('Built jujud.exe')


def create_cloud_agent(version, jujud_cmd_dir, ci_dir):
    tarball_name = 'juju-{}-win2012-amd64.tgz'.format(version)
    tarball_path = os.path.join(ci_dir, tarball_name)
    agent_path = os.path.join(jujud_cmd_dir, 'jujud.exe')
    with tarfile.open(name=tarball_path, mode='w:gz') as tar:
        tar.add(agent_path, arcname='jujud.exe')


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
        enable_cross_compile(GCC_BIN_DIR, GO_SRC_DIR, GOPATH)
        build_client(JUJU_CMD_DIR, GO_CMD, GOPATH, ISS_DIR)
        installer_name = create_installer(version, ISS_DIR, ISS_CMD, CI_DIR)
        install(installer_name)
        test(version)
        if has_agent(version):
            build_agent(JUJUD_CMD_DIR, GO_CMD, GOPATH)
            create_cloud_agent(version, JUJUD_CMD_DIR, CI_DIR)
        return 0
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
