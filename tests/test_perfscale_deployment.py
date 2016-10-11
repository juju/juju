"""Tests for assess_perf_test_simple module."""

import argparse
from mock import patch, Mock
import StringIO

from fakejuju import fake_juju_client
import perfscale_deployment as pd
from tests import (
    parse_error,
    TestCase,
)


default_args = argparse.Namespace(
    env='an-env',
    juju_bin='/bin/juju',
    logs='/tmp/logs',
    temp_env_name='an-env-mod',
    bundle_name='cs:~landscape/bundle/landscape-scalable',
    debug=False,
    agent_stream=None,
    agent_url=None,
    bootstrap_host=None,
    keep_env=False,
    machine=[],
    region=None,
    series=None,
    upload_tools=False,
    verbose=20,
    deadline=None,
)


class TestParseArgs(TestCase):

    def test_common_args(self):
        args = pd.parse_args(
            [
                'an-env',
                '/bin/juju',
                '/tmp/logs',
                'an-env-mod'])
        self.assertEqual(
            args,
            default_args
        )

    def test_default_bundle_name(self):
        args = pd.parse_args(
            ['an-env', '/bin/juju', '/tmp/logs', 'an-env-mod'])
        self.assertEqual(
            args.bundle_name, 'cs:~landscape/bundle/landscape-scalable')

    def test_passing_bundle_name(self):
        args = pd.parse_args(
            [
                'an-env',
                '/bin/juju',
                '/tmp/logs',
                'an-env-mod',
                '--bundle-name', 'cs:testing123'])
        self.assertEqual(
            args.bundle_name, 'cs:testing123')

    def test_help(self):
        fake_stdout = StringIO.StringIO()
        with parse_error(self) as fake_stderr:
            with patch('sys.stdout', fake_stdout):
                pd.parse_args(['--help'])
        self.assertEqual('', fake_stderr.getvalue())
        self.assertIn(
            'Perfscale bundle deployment test.', fake_stdout.getvalue())


class TestGetClientDetails(TestCase):

    def test_returns_expected_details(self):
        client = fake_juju_client()
        client.bootstrap()
        client.deploy('mongodb')
        self.assertEqual(
            dict(mongodb=1),
            pd.get_client_details(client))


class TestMain(TestCase):
    argv = ['an-env', '/bin/juju', '/tmp/logs', 'an-env-mod']
    bs_manager = Mock()
    with patch.object(pd, 'run_perfscale_test') as mock_run_pt:
        with patch.object(pd.BootstrapManager, 'from_args',
                          return_value=bs_manager):
            pd.main(argv)
    mock_run_pt.assert_called_once_with(
        pd.assess_deployment_perf,
        bs_manager,
        default_args)
