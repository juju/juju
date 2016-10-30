"""Tests for perfscale_mass_model_desctruction"""

import argparse
from mock import patch, Mock

import perfscale_mass_model_destruction as pmmd
from generate_perfscale_results import DeployDetails
from fakejuju import fake_juju_client
from tests import (
    TestCase
)
from utility import temp_dir


def get_default_args(model_count=100, log_dir='/tmp/logs'):
    return argparse.Namespace(
        env='an-env',
        juju_bin='/bin/juju',
        logs=log_dir,
        temp_env_name='an-env-mod',
        model_count=model_count,
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


class TestPerfscaleAssessModelDestruction(TestCase):

    def test_returns_DeployDetails(self):
        client = fake_juju_client()
        client.bootstrap()

        args = argparse.Namespace(model_count=1)

        with patch.object(pmmd, 'sleep', autospec=True):
            results = pmmd.perfscale_assess_model_destruction(client, args)
        self.assertIsInstance(results, DeployDetails)

    def test_returns_creates_requested_model_amount(self):
        client = fake_juju_client()
        client.bootstrap()

        args = argparse.Namespace(model_count=12)

        with patch.object(pmmd, 'sleep', autospec=True):
            results = pmmd.perfscale_assess_model_destruction(client, args)
        self.assertEqual(results.applications['Model Count'], 12)


class TestParseArgs(TestCase):

    def test_default_args(self):
        expected_args = get_default_args(model_count=42)
        self.assertEqual(
            expected_args,
            pmmd.parse_args(
                ['an-env',
                 '/bin/juju',
                 '/tmp/logs',
                 'an-env-mod',
                 '--model-count',
                 '42']))


class TestMain(TestCase):

    def test_main(self):
        with temp_dir() as log_dir:
            argv = ['an-env', '/bin/juju', log_dir, 'an-env-mod']
            bs_manager = Mock()
            with patch.object(pmmd, 'run_perfscale_test') as mock_run_pt:
                with patch.object(pmmd.BootstrapManager, 'from_args',
                                  return_value=bs_manager):
                    pmmd.main(argv)
            mock_run_pt.assert_called_once_with(
                pmmd.perfscale_assess_model_destruction,
                bs_manager,
                get_default_args(log_dir=log_dir))
