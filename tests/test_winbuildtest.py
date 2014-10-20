from contextlib import contextmanager
from mock import patch
import os
import shutil
import tarfile
from tempfile import mkdtemp
from unittest import TestCase

import winbuildtest
from winbuildtest import (
    build_agent,
    build_client,
    create_cloud_agent,
    GO_CMD,
    GOPATH,
)


@contextmanager
def temp_path(module, attr):
    dir_name = mkdtemp()
    old_path = getattr(module, attr)
    if os.path.isabs(old_path):
        rel_path = old_path[1:]
    else:
        rel_path = old_path
    new_path = os.path.join(dir_name, rel_path)
    os.makedirs(new_path)
    try:
        setattr(module, attr, new_path)
        yield new_path
    finally:
        setattr(module, attr, old_path)
        shutil.rmtree(dir_name)


@contextmanager
def temp_dir():
    dirname = mkdtemp()
    try:
        yield dirname
    finally:
        shutil.rmtree(dirname)


class WinBuildTestTestCase(TestCase):

    def test_build_client(self):
        with temp_dir() as cmd_dir:
            with temp_dir() as iss_dir:

                def make_juju(*args, **kwargs):
                    with open('%s/juju.exe' % cmd_dir, 'w') as fake_juju:
                        fake_juju.write('juju')

                with patch('winbuildtest.run',
                           return_value='', side_effect=make_juju) as run_mock:
                    build_client(cmd_dir, GO_CMD, GOPATH, iss_dir)
                    args, kwargs = run_mock.call_args
                    self.assertEqual((GO_CMD, 'build'), args)
                    self.assertEqual('386', kwargs['env'].get('GOARCH'))
                    self.assertEqual(GOPATH, kwargs['env'].get('GOPATH'))

    def test_build_agent(self):
        with temp_dir() as jujud_cmd_dir:
            with patch('winbuildtest.run', return_value='') as run_mock:
                build_agent(jujud_cmd_dir, GO_CMD, GOPATH)
                args, kwargs = run_mock.call_args
                self.assertEqual((GO_CMD, 'build'), args)
                self.assertEqual('amd64', kwargs['env'].get('GOARCH'))
                self.assertEqual(GOPATH, kwargs['env'].get('GOPATH'))

    def test_create_cloud_agent(self):
        with temp_path(winbuildtest, 'CI_DIR') as ci_dir:
            with temp_path(winbuildtest, 'JUJUD_CMD_DIR') as cmd_dir:
                with open('%s/jujud.exe' % cmd_dir, 'w') as fake_jujud:
                    fake_jujud.write('jujud')
                create_cloud_agent('1.20.1')
                agent = '{}/juju-1.20.1-win2012-amd64.tgz'.format(ci_dir)
                self.assertTrue(os.path.isfile(agent))
                with tarfile.open(name=agent, mode='r:gz') as tar:
                    self.assertEqual(['jujud.exe'], tar.getnames())
