from mock import patch
import os
import tarfile
from unittest import TestCase

from winbuildtest import (
    build_agent,
    build_client,
    create_cloud_agent,
    create_installer,
    enable_cross_compile,
    has_agent,
    GO_CMD,
    GOPATH,
    ISS_CMD,
)
from utils import temp_dir


class WinBuildTestTestCase(TestCase):

    def test_has_agent(self):
        self.assertFalse(has_agent('1.20.11'))
        self.assertTrue(has_agent('1.21-alpha3'))
        self.assertTrue(has_agent('1.21.0'))
        self.assertTrue(has_agent('1.22.0'))

    def test_enable_cross_compile(self):
        with temp_dir() as gcc_bin_dir:
            with temp_dir() as go_src_dir:
                with patch('winbuildtest.run', return_value='') as run_mock:
                    devnull = open(os.devnull, 'w')
                    with patch('sys.stdout', devnull):
                        enable_cross_compile(gcc_bin_dir, go_src_dir, GOPATH)
        expected_args = ('make.bat', '--no-clean')
        # The first call set the GOARCH to amd64
        output, args, kwargs = run_mock.mock_calls[0]
        self.assertEqual(expected_args, args)
        paths = kwargs['env'].get('PATH').split(os.pathsep)
        self.assertEqual(gcc_bin_dir, paths[-1])
        self.assertEqual(GOPATH, kwargs['env'].get('GOPATH'))
        self.assertEqual('amd64', kwargs['env'].get('GOARCH'))
        # The second call set the GOARCH to 386
        output, args, kwargs = run_mock.mock_calls[1]
        self.assertEqual(expected_args, args)
        paths = kwargs['env'].get('PATH').split(os.pathsep)
        self.assertEqual(gcc_bin_dir, paths[-1])
        self.assertEqual(GOPATH, kwargs['env'].get('GOPATH'))
        self.assertEqual('386', kwargs['env'].get('GOARCH'))

    def test_build_client(self):
        # build_client() builds the juju client with go and moved the
        # exe to the iss dir.
        with temp_dir() as cmd_dir:
            with temp_dir() as iss_dir:

                def make_juju(*args, **kwargs):
                    with open('%s/juju.exe' % cmd_dir, 'w') as fake_juju:
                        fake_juju.write('juju')

                with patch('winbuildtest.run',
                           return_value='', side_effect=make_juju) as run_mock:
                    devnull = open(os.devnull, 'w')
                    with patch('sys.stdout', devnull):
                        build_client(cmd_dir, GO_CMD, GOPATH, iss_dir)
                        args, kwargs = run_mock.call_args
                        self.assertEqual((GO_CMD, 'build'), args)
                        self.assertEqual('386', kwargs['env'].get('GOARCH'))
                        self.assertEqual(GOPATH, kwargs['env'].get('GOPATH'))
                        client_path = os.path.join(iss_dir, 'juju.exe')
                        self.assertTrue(os.path.isfile(client_path))

    def test_create_installer(self):
        # create_installer() creates an iss-style installer and copies it
        # to the ci dir.
        with temp_dir() as iss_dir:
            with temp_dir() as ci_dir:
                installer_name = 'juju-setup-1.20.1.exe'

                def make_installer(*args, **kwargs):
                    output_dir = os.path.join(iss_dir, 'output')
                    os.makedirs(output_dir)
                    installer_path = os.path.join(
                        output_dir, installer_name)
                    with open(installer_path, 'w') as fake_installer:
                        fake_installer.write('juju installer')

                with patch('winbuildtest.run',
                           return_value='',
                           side_effect=make_installer) as run_mock:
                    devnull = open(os.devnull, 'w')
                    with patch('sys.stdout', devnull):
                        create_installer('1.20.1', iss_dir, ISS_CMD, ci_dir)
                        args, kwargs = run_mock.call_args
                        self.assertEqual((ISS_CMD, 'setup.iss'), args)
                        installer_path = os.path.join(ci_dir, installer_name)
                        self.assertTrue(os.path.isfile(installer_path))

    def test_build_agent(self):
        # build_agent creates a win amd64 jujud.
        with temp_dir() as jujud_cmd_dir:
            with patch('winbuildtest.run', return_value='') as run_mock:
                devnull = open(os.devnull, 'w')
                with patch('sys.stdout', devnull):
                    build_agent(jujud_cmd_dir, GO_CMD, GOPATH)
                    args, kwargs = run_mock.call_args
                    self.assertEqual((GO_CMD, 'build'), args)
                    self.assertEqual('amd64', kwargs['env'].get('GOARCH'))
                    self.assertEqual(GOPATH, kwargs['env'].get('GOPATH'))

    def test_create_cloud_agent(self):
        # create_cloud_agent() creates an agent tgz from the jujud and
        # copies it to the ci dir.
        with temp_dir() as cmd_dir:
            with temp_dir() as ci_dir:
                with open('%s/jujud.exe' % cmd_dir, 'w') as fake_jujud:
                    fake_jujud.write('jujud')
                create_cloud_agent('1.20.1', cmd_dir, ci_dir)
                agent = os.path.join(ci_dir, 'juju-1.20.1-win2012-amd64.tgz')
                self.assertTrue(os.path.isfile(agent))
                with tarfile.open(name=agent, mode='r:gz') as tar:
                    self.assertEqual(['jujud.exe'], tar.getnames())
