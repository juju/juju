from argparse import Namespace
import logging
import os
import pickle
import stat
import subprocess
from tempfile import NamedTemporaryFile
import unittest

from mock import (
    call,
    Mock,
    patch,
)

from jujupy import (
    EnvJujuClient,
    JujuData,
    )
from run_deployer import (
    apply_condition,
    assess_deployer,
    check_health,
    CLOCK_SKEW_SCRIPT,
    ErrUnitCondition,
    main,
    parse_args,
    )
import tests
from tests.test_jujupy import FakeJujuClient


class TestParseArgs(tests.TestCase):

    def test_parse_args(self):
        args = parse_args(['/bundle/path', 'test_env', 'new/bin/juju',
                           '/tmp/logs', 'test_job'])
        self.assertEqual(args.bundle_path, '/bundle/path')
        self.assertEqual(args.env, 'test_env')
        self.assertEqual(args.juju_bin, 'new/bin/juju')
        self.assertEqual(args.logs, '/tmp/logs')
        self.assertEqual(args.temp_env_name, 'test_job')
        self.assertEqual(args.allow_native_deploy, False)
        self.assertEqual(args.bundle_name, None)
        self.assertEqual(args.health_cmd, None)
        self.assertEqual(args.keep_env, False)
        self.assertEqual(args.agent_url, None)
        self.assertEqual(args.agent_stream, None)
        self.assertEqual(args.series, None)
        self.assertEqual(args.debug, False)
        self.assertEqual(args.verbose, logging.INFO)
        self.assertEqual(args.upgrade, False)
        self.assertEqual(args.upgrade_condition, None)
        self.assertEqual(args.agent_timeout, 1200)
        self.assertEqual(args.workload_timeout, 1800)

    def test_allow_native_bundle(self):
        args = parse_args(['./bundle/path', 'an_env', './juju', './logs',
                           'temp_env', '--allow-native-deploy'])
        self.assertEqual(args.bundle_path, './bundle/path')
        self.assertEqual(args.env, 'an_env')
        self.assertEqual(args.juju_bin, './juju')
        self.assertEqual(args.logs, './logs')
        self.assertEqual(args.temp_env_name, 'temp_env')
        self.assertEqual(args.allow_native_deploy, True)

    def test_native_bundle_no_name(self):
        with tests.parse_error(self) as stderr:
            parse_args(['./bundle/path', 'an_env', './juju', './logs',
                        'temp_env', '--allow-native-deploy',
                        '--bundle-name', 'specific_bundle'])
        self.assertRegexpMatches(
            stderr.getvalue(),
            'error: cannot supply bundle name with native juju deploying$')


class TestMain(tests.FakeHomeTestCase):

    def test_basic_args(self):
        args = ['bundles', 'an-env', '/bin/juju', 'logs', 'deployer-env']
        env = JujuData('an-env')
        client = EnvJujuClient(env, '1.234-76', None)
        with patch('jujupy.SimpleEnvironment.from_config',
                   return_value=env) as e_mock:
            with patch('jujupy.EnvJujuClient.by_version',
                       return_value=client) as c_mock:
                with patch('run_deployer.boot_context'):
                    with patch('run_deployer.assess_deployer') as ad_mock:
                        main(args)
        e_mock.assert_called_once_with('an-env')
        c_mock.assert_called_once_with(env, '/bin/juju', debug=False)
        ad_mock.assert_called_once_with(parse_args(args), client, 1200, 1800)

    def test_basic_args_native_deploy(self):
        args = ['mediawiki-scalable.yaml', 'an-env', '/bin/juju', 'logs',
                'deployer-env', '--allow-native-deploy',
                '--bundle-verification-script',
                'verify_mediawiki_bundle.py']
        env = JujuData('an-env')
        client = EnvJujuClient(env, '1.234-76', None)
        with patch('jujupy.SimpleEnvironment.from_config',
                   return_value=env) as e_mock:
            with patch('jujupy.EnvJujuClient.by_version',
                       return_value=client) as c_mock:
                with patch('run_deployer.boot_context'):
                    with patch('run_deployer.assess_deployer') as ad_mock:
                        with patch('run_deployer.run_command') as mb_mock:
                            main(args)
        e_mock.assert_called_once_with('an-env')
        c_mock.assert_called_once_with(env, '/bin/juju', debug=False)
        ad_mock.assert_called_once_with(parse_args(args), client, 1200, 1800)
        client_ser = pickle.dumps(client)
        mb_mock.assert_called_once_with(['verify_mediawiki_bundle.py',
                                         client_ser])

    def test_basic_args_native_deploy_landscape(self):
        args = ['cs:~landscape/bundle/landscape-scalable', 'an-env',
                '/bin/juju', 'logs', 'deployer-env',
                '--allow-native-deploy',
                '--bundle-verification-script',
                'verify_landscape_bundle.py']
        env = JujuData('an-env')
        client = EnvJujuClient(env, '1.234-76', None)
        with patch('jujupy.SimpleEnvironment.from_config',
                   return_value=env) as e_mock:
            with patch('jujupy.EnvJujuClient.by_version',
                       return_value=client) as c_mock:
                with patch('run_deployer.boot_context'):
                    with patch('run_deployer.assess_deployer') as ad_mock:
                            with patch('run_deployer.run_command') as rc:
                                main(args)
        e_mock.assert_called_once_with('an-env')
        c_mock.assert_called_once_with(env, '/bin/juju', debug=False)
        ad_mock.assert_called_once_with(parse_args(args), client, 1200, 1800)
        client_ser = pickle.dumps(client)
        rc.assert_called_once_with(['verify_landscape_bundle.py',
                                   client_ser])


class TestAssessDeployer(tests.TestCase):

    @staticmethod
    def make_args(temp_env_name='foo', env='bar', series=None, agent_url=None,
                  agent_stream=None, juju_bin='', logs=None, keep_env=False,
                  health_cmd=None, debug=False, bundle_path='bundle.yaml',
                  bundle_name='bu', verbose=logging.INFO, region=None,
                  upgrade=False, upgrade_condition=None,
                  allow_native_deploy=False):
        return Namespace(
            temp_env_name=temp_env_name, env=env, series=series,
            agent_url=agent_url, agent_stream=agent_stream, juju_bin=juju_bin,
            logs=logs, keep_env=keep_env, health_cmd=health_cmd, debug=debug,
            bundle_path=bundle_path, bundle_name=bundle_name, verbose=verbose,
            region=region, upgrade=upgrade,
            allow_native_deploy=allow_native_deploy,
            upgrade_condition=upgrade_condition)

    def test_health(self):
        args = self.make_args(health_cmd='/tmp/check')
        client_mock = Mock(spec=EnvJujuClient)
        with patch('run_deployer.check_health', autospec=True) as ch_mock:
            assess_deployer(args, client_mock, 600, 1800)
        client_mock.deployer.assert_called_once_with('bundle.yaml', 'bu')
        client_mock.wait_for_workloads.assert_called_once_with(timeout=1800)
        environ = client_mock._shell_environ()
        ch_mock.assert_called_once_with('/tmp/check', 'foo', environ)

    def test_upgrade(self):
        args = self.make_args(juju_bin='new/juju', upgrade=True)
        client_mock = Mock(spec=EnvJujuClient)
        with patch('run_deployer.assess_upgrade', autospec=True) as au_mock:
            assess_deployer(args, client_mock, 600, 1800)
        client_mock.deployer.assert_called_once_with('bundle.yaml', 'bu')
        client_mock.show_status.assert_called_once_with()
        au_mock.assert_called_once_with(client_mock, 'new/juju')
        self.assertEqual(
            client_mock.wait_for_workloads.call_args_list,
            [call(timeout=1800), call()])

    def test_upgrade_and_health(self):
        args = self.make_args(health_cmd='/tmp/check', juju_bin='new/juju',
                              upgrade=True)
        client_mock = Mock(spec=EnvJujuClient)
        with patch('run_deployer.assess_upgrade', autospec=True) as au_mock:
            with patch('run_deployer.check_health', autospec=True) as ch_mock:
                assess_deployer(args, client_mock, 600, 1800)
        client_mock.deployer.assert_called_once_with('bundle.yaml', 'bu')
        client_mock.show_status.assert_called_once_with()
        au_mock.assert_called_once_with(client_mock, 'new/juju')
        self.assertEqual(
            client_mock.wait_for_workloads.call_args_list,
            [call(timeout=1800), call()])
        environ = client_mock._shell_environ()
        self.assertEqual(
            ch_mock.call_args_list, [call('/tmp/check', 'foo', environ)] * 2)

    @patch('run_deployer.SimpleEnvironment.from_config')
    @patch('run_deployer.boot_context', autospec=True)
    def test_run_deployer_upgrade(self, *args):
        args = self.make_args(
            juju_bin='baz/juju', upgrade=True,
            upgrade_condition=['bla/0:clock_skew', 'foo/1:fill_disk'])
        client = FakeJujuClient()
        client.bootstrap()
        with patch('run_deployer.EnvJujuClient.by_version',
                   return_value=client):
            with patch('run_deployer.apply_condition') as ac_mock:
                with patch('run_deployer.assess_upgrade') as au_mock:
                    assess_deployer(args, client, 600, 1800)
        self.assertEqual(2, ac_mock.call_count)
        self.assertEqual(
            ac_mock.call_args_list,
            [call(client, 'bla/0:clock_skew'),
             call(client, 'foo/1:fill_disk')])
        au_mock.assert_called_once_with(client, 'baz/juju')

    def test_allow_native_deploy(self):
        args = self.make_args(allow_native_deploy=True)
        client_mock = Mock(spec=EnvJujuClient)
        assess_deployer(args, client_mock, 600, 1800)
        client_mock.deploy_bundle.assert_called_once_with('bundle.yaml')
        client_mock.wait_for_started.assert_called_once_with(timeout=600)
        client_mock.wait_for_workloads.assert_called_once_with(timeout=1800)


class FakeRemote():
    """Fake remote class for testing."""

    def __init__(self):
        self.series = 'foo'

    def is_windows(self):
        return False

    def run(self, command):
        self.command = command


class TestApplyCondition(tests.TestCase):

    def test_apply_condition_clock_skew(self):
        client = FakeJujuClient()
        remote = FakeRemote()
        with patch('run_deployer.remote_from_unit',
                   return_value=remote, autospec=True) as ru_mock:
            apply_condition(client, 'bla/0:clock_skew')
        ru_mock.assert_called_once_with(client, 'bla/0')
        self.assertEqual(CLOCK_SKEW_SCRIPT, remote.command)

    def test_apply_condition_raises_ErrUnitCondition(self):
        client = FakeJujuClient()
        remote = FakeRemote()
        with patch('run_deployer.remote_from_unit',
                   return_value=remote) as rfu_mock:
            with self.assertRaises(ErrUnitCondition):
                apply_condition(client, 'bla/0:foo')
            rfu_mock.assert_called_once_with(client, 'bla/0')


class TestIsHealthy(unittest.TestCase):

    def test_check_health(self):
        SCRIPT = """#!/bin/bash\necho -n 'PASS'\nexit 0"""
        with NamedTemporaryFile(delete=False) as health_script:
            health_script.write(SCRIPT)
            os.fchmod(health_script.fileno(), stat.S_IEXEC | stat.S_IREAD)
            health_script.close()
            with patch('logging.info') as lo_mock:
                check_health(health_script.name)
            os.unlink(health_script.name)
            self.assertEqual(lo_mock.call_args[0][0],
                             'Health check output: PASS')

    def test_check_health_with_env_name(self):
        SCRIPT = """#!/bin/bash\necho -n \"PASS on $1\"\nexit 0"""
        with NamedTemporaryFile(delete=False) as health_script:
            health_script.write(SCRIPT)
            os.fchmod(health_script.fileno(), stat.S_IEXEC | stat.S_IREAD)
            health_script.close()
            with patch('logging.info') as lo_mock:
                check_health(health_script.name, 'foo')
            os.unlink(health_script.name)
            self.assertEqual(lo_mock.call_args[0][0],
                             'Health check output: PASS on foo')

    def test_check_health_fail(self):
        SCRIPT = """#!/bin/bash\necho -n 'FAIL'\nexit 1"""
        with NamedTemporaryFile(delete=False) as health_script:
            health_script.write(SCRIPT)
            os.fchmod(health_script.fileno(), stat.S_IEXEC | stat.S_IREAD)
            health_script.close()
            with patch('logging.error') as le_mock:
                with self.assertRaises(subprocess.CalledProcessError):
                    check_health(health_script.name)
            os.unlink(health_script.name)
            self.assertEqual(le_mock.call_args[0][0], 'FAIL')

    def test_check_health_with_no_execute_perms(self):
        SCRIPT = """#!/bin/bash\nexit 0"""
        with NamedTemporaryFile(delete=False) as health_script:
            health_script.write(SCRIPT)
            os.fchmod(health_script.fileno(), stat.S_IREAD)
            health_script.close()
            with patch('logging.error') as le_mock:
                with self.assertRaises(OSError):
                    check_health(health_script.name)
            os.unlink(health_script.name)
        self.assertRegexpMatches(
            le_mock.call_args[0][0],
            r'Failed to execute.*: \[Errno 13\].*')
