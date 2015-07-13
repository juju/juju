#!/usr/bin/env python
from argparse import ArgumentParser
import os
import subprocess
from textwrap import dedent
import yaml


def win_test(script_dir, address, juju_home):
    host = 'Administrator@{}'.format(address)
    private_key = os.path.join(juju_home, 'staging-juju-rsa')
    install_file = subprocess.check_output([
        os.path.join(script_dir, 'jujuci.py'), 'get', 'build-win-client',
        'juju-setup-*.exe', './']).rstrip('\n')
    with open('run-file', 'w') as run_file:
        run_file.write(dedent("""
            ci/$1 /verysilent
            juju version
            juju destroy-environment --force -y test-win-client
            python ci\\\\deploy_stack.py test-win-client
            """))

    ci = [os.path.join(script_dir, f) for f in [
        'deploy_stack.py', 'jujupy.py', 'jujuconfig.py', 'remote.py',
        'substrate.py', 'utility.py', 'get_ami.py',
        ]]
    ci.extend([install_file, 'run-file'])
    with open('foo.yaml', 'w') as config:
        yaml.dump({
            'install': {'ci': ci},
            'command': ['ci/run-file', install_file],
            }, config)
    subprocess.check_call(['workspace-run', '-v', 'foo.yaml', host, '-i',
                           private_key])


def main():
    parser = ArgumentParser()
    parser.add_argument('address',
                        help='The IP or DNS address the windows test machine.')
    parser.add_argument(
        '--juju-home', default=os.environ.get('JUJU_HOME'),
        help='The location of cloud-city and staging-juju-rsa.')
    script_dir = os.path.dirname(__file__)
    win_test(script_dir=script_dir, **parser.parse_args().__dict__)


if __name__ == '__main__':
    main()
