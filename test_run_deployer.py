from argparse import Namespace
import os
import stat
from tempfile import NamedTemporaryFile
from unittest import TestCase

from mock import patch

from jujupy import (
    EnvJujuClient,
    SimpleEnvironment,
    )
from run_deployer import (
    is_healthy,
    parse_args,
    run_deployer
    )


class TestParseArgs(TestCase):

    def test_parse_args(self):
        args = parse_args(['/bundle/path', 'test_env', '/tmp/logs',
                           'test_job'])
        self.assertEqual(args.bundle_path, '/bundle/path')
        self.assertEqual(args.env, 'test_env')
        self.assertEqual(args.logs, '/tmp/logs')
        self.assertEqual(args.job_name, 'test_job')
        self.assertEqual(args.bundle_name, None)
        self.assertEqual(args.health_cmd, None)
        self.assertEqual(args.keep_env, False)
        self.assertEqual(args.agent_url, None)
        self.assertEqual(args.agent_stream, None)
        self.assertEqual(args.series, None)
        self.assertEqual(args.debug, False)
        self.assertEqual(args.verbose, False)
        self.assertEqual(args.new_juju_bin, None)


class TestRunDeployer(TestCase):

    def test_run_deployer(self):
        with patch('run_deployer.boot_context'):
            with patch('run_deployer.SimpleEnvironment.from_config',
                       return_value=SimpleEnvironment('bar')) as env:
                with patch('run_deployer.EnvJujuClient.by_version',
                           return_value=EnvJujuClient(env, '1.234-76', None)):
                    with patch('run_deployer.parse_args',
                               return_value=Namespace(
                                   job_name='foo', env='bar', series=None,
                                   agent_url=None, agent_stream=None,
                                   new_juju_bin='', logs=None, keep_env=False,
                                   health_cmd=None, debug=False,
                                   bundle_path='', bundle_name='')):
                        with patch(
                                'run_deployer.EnvJujuClient.deployer') as dm:
                            with patch('run_deployer.is_healthy') as hm:
                                run_deployer()
        self.assertEqual(dm.call_count, 1)
        self.assertEqual(hm.call_count, 0)

    def test_run_deployer_health(self):
        with patch('run_deployer.boot_context'):
            with patch('run_deployer.SimpleEnvironment.from_config',
                       return_value=SimpleEnvironment('bar')) as env:
                with patch('run_deployer.EnvJujuClient.by_version',
                           return_value=EnvJujuClient(env, '1.234-76', None)):
                    with patch('run_deployer.parse_args',
                               return_value=Namespace(
                                   job_name='foo', env='bar', series=None,
                                   agent_url=None, agent_stream=None,
                                   new_juju_bin='', logs=None, keep_env=False,
                                   health_cmd='/tmp/check', debug=False,
                                   bundle_path='', bundle_name='')):
                        with patch('run_deployer.EnvJujuClient.deployer'):
                            with patch('run_deployer.is_healthy') as hm:
                                run_deployer()
        self.assertEqual(hm.call_count, 1)


class TestIsHealthy(TestCase):

    def test_is_healthy(self):
        SCRIPT = """#!/bin/bash\necho -n 'PASS'\nexit 0"""
        with NamedTemporaryFile(delete=False) as health_script:
            health_script.write(SCRIPT)
            os.fchmod(health_script.fileno(), stat.S_IEXEC | stat.S_IREAD)
            health_script.close()
            with patch('logging.info') as lo_mock:
                result = is_healthy(health_script.name)
            os.unlink(health_script.name)
            self.assertTrue(result)
            self.assertEqual(lo_mock.call_args[0][0],
                             'Health check output: PASS')

    def test_is_healthy_fail(self):
        SCRIPT = """#!/bin/bash\necho -n 'FAIL'\nexit 1"""
        with NamedTemporaryFile(delete=False) as health_script:
            health_script.write(SCRIPT)
            os.fchmod(health_script.fileno(), stat.S_IEXEC | stat.S_IREAD)
            health_script.close()
            with patch('logging.error') as le_mock:
                result = is_healthy(health_script.name)
            os.unlink(health_script.name)
            self.assertFalse(result)
            self.assertEqual(le_mock.call_args[0][0], 'FAIL')

    def test_is_healthy_with_no_execute_perms(self):
        SCRIPT = """#!/bin/bash\nexit 0"""
        with NamedTemporaryFile(delete=False) as health_script:
            health_script.write(SCRIPT)
            os.fchmod(health_script.fileno(), stat.S_IREAD)
            health_script.close()
            with patch('logging.error') as le_mock:
                with self.assertRaises(OSError):
                    is_healthy(health_script.name)
            os.unlink(health_script.name)
        self.assertRegexpMatches(
            le_mock.call_args[0][0],
            r'The health check script failed to execute with: \[Errno 13\].*')
