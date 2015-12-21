from argparse import Namespace
import logging
import os
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
    SimpleEnvironment,
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
from tests.test_jujupy import FakeJujuClient


import tests


class TestParseArgs(tests.TestCase):

    def test_parse_args(self):
        args = parse_args(['/bundle/path', 'test_env', 'new/bin/juju',
                           '/tmp/logs', 'test_job'])
        self.assertEqual(args.bundle_path, '/bundle/path')
        self.assertEqual(args.env, 'test_env')
        self.assertEqual(args.juju_bin, 'new/bin/juju')
        self.assertEqual(args.logs, '/tmp/logs')
        self.assertEqual(args.temp_env_name, 'test_job')
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


class TestMain(tests.FakeHomeTestCase):

    def test_basic_args(self):
        args = ['bundles', 'an-env', '/bin/juju', 'logs', 'deployer-env']
        env = SimpleEnvironment('an-env')
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
        ad_mock.assert_called_once_with(parse_args(args), client)


class TestAssessDeployer(tests.TestCase):

    def test_health(self):
        args = Namespace(
            temp_env_name='foo', env='bar', series=None, agent_url=None,
            agent_stream=None, juju_bin='', logs=None, keep_env=False,
            health_cmd='/tmp/check', debug=False, bundle_path='bundle.yaml',
            bundle_name='bu', verbose=logging.INFO, region=None, upgrade=False)
        client_mock = Mock(spec=EnvJujuClient)
        with patch('run_deployer.check_health', autospec=True) as ch_mock:
            assess_deployer(args, client_mock)
        client_mock.deployer.assert_called_once_with('bundle.yaml', 'bu')
        # Curtis disabled this because no deployer-based tests are passing.
        # client_mock.wait_for_workloads.assert_called_once_with()
        ch_mock.assert_called_once_with('/tmp/check', 'foo')

    def test_upgrade(self):
        args = Namespace(
            temp_env_name='foo', env='bar', series=None, agent_url=None,
            agent_stream=None, juju_bin='new/juju', logs=None, keep_env=False,
            health_cmd=None, debug=False, bundle_path='bundle.yaml',
            bundle_name='bu', verbose=logging.INFO, region=None, upgrade=True,
            upgrade_condition=[])
        client_mock = Mock(spec=EnvJujuClient)
        with patch('run_deployer.assess_upgrade', autospec=True) as au_mock:
            assess_deployer(args, client_mock)
        client_mock.deployer.assert_called_once_with('bundle.yaml', 'bu')
        client_mock.juju.assert_called_once_with('status', ())
        au_mock.assert_called_once_with(client_mock, 'new/juju')
        # Curtis disabled this because no deployer-based tests are passing.
        # self.assertEqual(
        #     client_mock.wait_for_workloads.call_args_list, [call()] * 2)

    def test_upgrade_and_health(self):
        args = Namespace(
            temp_env_name='foo', env='bar', series=None, agent_url=None,
            agent_stream=None, juju_bin='new/juju', logs=None, keep_env=False,
            health_cmd='/tmp/check', debug=False, bundle_path='bundle.yaml',
            bundle_name='bu', verbose=logging.INFO, region=None, upgrade=True,
            upgrade_condition=[])
        client_mock = Mock(spec=EnvJujuClient)
        with patch('run_deployer.assess_upgrade', autospec=True) as au_mock:
            with patch('run_deployer.check_health', autospec=True) as ch_mock:
                assess_deployer(args, client_mock)
        client_mock.deployer.assert_called_once_with('bundle.yaml', 'bu')
        client_mock.juju.assert_called_once_with('status', ())
        au_mock.assert_called_once_with(client_mock, 'new/juju')
        # Curtis disabled this because no deployer-based tests are passing.
        # self.assertEqual(
        #     client_mock.wait_for_workloads.call_args_list, [call()] * 2)
        self.assertEqual(
            ch_mock.call_args_list, [call('/tmp/check', 'foo')] * 2)

    @patch('run_deployer.SimpleEnvironment.from_config')
    @patch('run_deployer.boot_context', autospec=True)
    def test_run_deployer_upgrade(self, *args):
        args = Namespace(
            env='foo', juju_bin='baz/juju', logs=None, temp_env_name='foo_t',
            bundle_path='bundle.yaml', bundle_name='bundle',
            health_cmd=None, debug=False, upgrade=True,
            upgrade_condition=['bla/0:clock_skew', 'foo/1:fill_disk'])
        client = FakeJujuClient()
        with patch('run_deployer.EnvJujuClient.by_version',
                   return_value=client):
            with patch('run_deployer.apply_condition') as ac_mock:
                with patch('run_deployer.assess_upgrade') as au_mock:
                    assess_deployer(args, client)
        self.assertEqual(2, ac_mock.call_count)
        args, kwargs = ac_mock.call_args_list[0]
        self.assertEqual(args[0], client)
        self.assertEqual(args[1], 'bla/0:clock_skew')
        args, kwargs = ac_mock.call_args_list[1]
        self.assertEqual(args[0], client)
        self.assertEqual(args[1], 'foo/1:fill_disk')
        au_mock.assert_called_once_with(client, 'baz/juju')


class FakeRemote():

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
                   return_value=remote) as ru_mock:
            apply_condition(client, 'bla/0:clock_skew')
        self.assertEqual(ru_mock.call_count, 1)
        ru_mock.assert_called_once_with(client, 'bla/0')
        self.assertEqual(CLOCK_SKEW_SCRIPT, remote.command)

    def test_apply_condition_raises_ErrUnitCondition(self):
        client = FakeJujuClient()
        remote = FakeRemote()
        with patch('run_deployer.remote_from_unit', return_value=remote):
            with self.assertRaises(ErrUnitCondition):
                apply_condition(client, 'bla/0:foo')


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
