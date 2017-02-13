"""Tests for perfscale_controller_stress module."""

from datetime import datetime, timedelta
from mock import patch, Mock
import StringIO

import perfscale_controller_stress as pcs
from tests import (
    parse_error,
    TestCase,
)
from generate_perfscale_results import (
    DeployDetails,
    TimingData,
)
from test_generate_perfscale_results import (
    get_default_args,
)
from utility import temp_dir


def _get_default_args(**kwargs):
    # Wrap default args for this test.
    amount = kwargs.pop('deploy_amount', 1)
    return get_default_args(deploy_amount=amount, **kwargs)


class TestAssessControllerStress(TestCase):

    def test_returns_DeployDetails(self):
        args = _get_default_args(deploy_amount=0)
        client = Mock()
        pprof_collector = Mock()

        deploy_details = pcs.assess_controller_stress(
            client, pprof_collector, args)

        self.assertIs(type(deploy_details), DeployDetails)
        self.assertEqual(deploy_details.name, 'Controller Stress.')

    def test_deploys_and_adds_units(self):
        args = _get_default_args(deploy_amount=1)
        client = Mock()
        pprof_collector = Mock()

        with patch.object(
                pcs, 'deploy_swarm_to_new_model', autospec=True) as m_dsnm:
            pcs.assess_controller_stress(client, pprof_collector, args)
        m_dsnm.assert_called_once_with(client, 'swarm-model-0')

    def test_stores_deploytimes(self):
        start = datetime(2016, 11, 1, 2, 17, 48)
        end = datetime(2016, 11, 1, 2, 17, 50)
        times = [start, end]
        comparison_timing = TimingData(start, end)
        args = _get_default_args(deploy_amount=0)
        client = Mock()
        pprof_collector = Mock()

        with patch.object(pcs, 'datetime', autospec=True) as m_dt:
            m_dt.utcnow.side_effect = times
            deploy_details = pcs.assess_controller_stress(
                client, pprof_collector, args)
        deploy_timing = deploy_details.timings
        self.assertEqual(deploy_timing.start, comparison_timing.start)
        self.assertEqual(deploy_timing.end, comparison_timing.end)


class DeploySwarmToNewModel(TestCase):

    def test_returns_elapsed_seconds(self):
        client = Mock()
        start = datetime.utcnow()
        end = start + timedelta(seconds=10)
        with patch.object(pcs, 'datetime', autospec=True) as m_dt:
            m_dt.utcnow.side_effect = [start, end]
            elapsed = pcs.deploy_swarm_to_new_model(client, 'testing')
        self.assertEqual(elapsed, int((end - start).total_seconds()))

    def test_deploys_charm_to_new_model(self):
        client = Mock()
        new_client = Mock()
        client.add_model.return_value = new_client

        pcs.deploy_swarm_to_new_model(client, 'testing')

        client.add_model.assert_called_once_with('testing')
        new_client.deploy.assert_called_once_with(
            'cs:~containers/observable-swarm')
        self.assertEqual(new_client.wait_for_started.call_count, 1)
        self.assertEqual(new_client.wait_for_workloads.call_count, 1)


class TestGetCharmUrl(TestCase):

    def test_returns_correct_url(self):
        self.assertEqual(
            pcs.get_charm_url(), 'cs:~containers/observable-swarm')


class TestParseArgs(TestCase):

    def test_common_args(self):
        args = pcs.parse_args(
            [
                'an-env',
                '/bin/juju',
                '/tmp/logs',
                'an-env-mod'])
        self.assertEqual(
            args,
            _get_default_args()
        )

    def test_default_deploy_amount(self):
        args = pcs.parse_args(
            ['an-env', '/bin/juju', '/tmp/logs', 'an-env-mod'])
        self.assertEqual(
            args.deploy_amount, 1)

    def test_passing_deploy_amount(self):
        args = pcs.parse_args(
            [
                'an-env',
                '/bin/juju',
                '/tmp/logs',
                'an-env-mod',
                '--deploy-amount', '42'])
        self.assertEqual(args.deploy_amount, 42)

    def test_help(self):
        fake_stdout = StringIO.StringIO()
        with parse_error(self) as fake_stderr:
            with patch('sys.stdout', fake_stdout):
                pcs.parse_args(['--help'])
        self.assertEqual('', fake_stderr.getvalue())
        self.assertIn(
            'Perfscale Controller Stress test.', fake_stdout.getvalue())


class TestMain(TestCase):
    with temp_dir() as log_dir:
        argv = ['an-env', '/bin/juju', log_dir, 'an-env-mod']
        bs_manager = Mock()
        with patch.object(pcs, 'run_perfscale_test') as mock_run_pt:
            with patch.object(pcs.BootstrapManager, 'from_args',
                              return_value=bs_manager):
                pcs.main(argv)
        mock_run_pt.assert_called_once_with(
            pcs.assess_controller_stress,
            bs_manager,
            _get_default_args(logs=log_dir))
