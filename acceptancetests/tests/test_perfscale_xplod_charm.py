"""Tests for perfscale_xplod_charm module."""

from datetime import datetime
from mock import call, patch, Mock
import StringIO
from types import GeneratorType

import perfscale_xplod_charm as pxc
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
    amount = kwargs.pop('deploy_amount', 7)
    return get_default_args(deploy_amount=amount, **kwargs)


class TestAssessXplodPerf(TestCase):

    def test_returns_DeployDetails(self):
        # Deploy zero so we don't have to add times for each deploy.
        args = _get_default_args(deploy_amount=0)
        client = Mock()
        pprof_collector = Mock()

        deploy_details = pxc.assess_xplod_perf(client, pprof_collector, args)

        self.assertIs(type(deploy_details), DeployDetails)
        self.assertEqual(deploy_details.name, 'Xplod charm')

    def test_deploys_and_adds_units(self):
        # Deploy zero so we don't have to add times for each deploy.
        args = _get_default_args(deploy_amount=0)
        client = Mock()
        pprof_collector = Mock()

        with patch.object(pxc, 'deploy_xplod_charm', autospec=True) as m_dxc:
            with patch.object(
                    pxc, 'add_multiple_units', autospec=True) as m_amu:
                pxc.assess_xplod_perf(client, pprof_collector, args)
        m_dxc.assert_called_once_with(client)
        m_amu.assert_called_once_with(client, args, pprof_collector)

    def test_stores_deploytimes(self):
        start = datetime(2016, 11, 1, 2, 17, 48)
        end = datetime(2016, 11, 1, 2, 17, 50)
        times = [start, end]
        comparison_timing = TimingData(start, end)
        # Deploy zero so we don't have to add times for each deploy.
        args = _get_default_args(deploy_amount=0)
        client = Mock()
        pprof_collector = Mock()

        with patch.object(pxc, 'datetime', autospec=True) as m_dt:
            m_dt.utcnow.side_effect = times
            deploy_details = pxc.assess_xplod_perf(
                client, pprof_collector, args)
        deploy_timing = deploy_details.timings
        self.assertEqual(deploy_timing.start, comparison_timing.start)
        self.assertEqual(deploy_timing.end, comparison_timing.end)


class TestDeployXplodCharm(TestCase):

    def test_deploys_xplod_charm(self):
        client = Mock()
        with patch.object(
                pxc, 'local_charm_path',
                autospec=True, return_value='/charm/path/xplod') as m_lcp:
            pxc.deploy_xplod_charm(client)
        client.deploy.assert_called_once_with(
            '/charm/path/xplod', series='trusty')
        client.wait_for_started.assert_called_once_with()
        client.wait_for_workloads.assert_called_once_with()
        m_lcp.assert_called_once_with(
            charm='peer-xplod', juju_ver=client.version)


class TestAddMultipleUnits(TestCase):

    def test_returns_deploy_results(self):
        client = Mock()
        pprof_collector = Mock()
        deploy_amount = 3
        args = _get_default_args(deploy_amount=deploy_amount)

        unit_deploys = pxc.add_multiple_units(client, args, pprof_collector)

        unit_keys = unit_deploys.keys()
        self.assertEqual(len(unit_keys), deploy_amount)
        self.assertItemsEqual(
            unit_keys,
            ['unit-1', 'unit-2', 'unit-3'])

    def test_adds_requested_unit_amount(self):
        client = Mock()
        pprof_collector = Mock()
        deploy_amount = 3
        args = _get_default_args(deploy_amount=deploy_amount)

        pxc.add_multiple_units(client, args, pprof_collector)

        self.assertItemsEqual(
            client.juju.call_args_list,
            [call('add-unit', ('peer-xplod', '-n', '1'))] * 3)
        self.assertEqual(client.wait_for_started.call_count, 3)
        self.assertEqual(client.wait_for_workloads.call_count, 3)
        self.assertEqual(pprof_collector.collect_profile.call_count, 3)


class TestSingularUnit(TestCase):

    def test_returns_generator(self):
        self.assertIs(type(pxc.singular_unit(1)), GeneratorType)

    def test_creates_generator_of_different_sizes(self):
        self.assertEqual(len(list(pxc.singular_unit(42))), 42)
        self.assertEqual(len(list(pxc.singular_unit(5))), 5)

    def test_generator_yields_correct_value(self):
        self.assertItemsEqual(
            list(pxc.singular_unit(5)),
            [1] * 5
        )


class TestParseArgs(TestCase):

    def test_common_args(self):
        args = pxc.parse_args(
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
        args = pxc.parse_args(
            ['an-env', '/bin/juju', '/tmp/logs', 'an-env-mod'])
        self.assertEqual(
            args.deploy_amount, 7)

    def test_passing_deploy_amount(self):
        args = pxc.parse_args(
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
                pxc.parse_args(['--help'])
        self.assertEqual('', fake_stderr.getvalue())
        self.assertIn(
            'Perfscale xplod charm stree test.', fake_stdout.getvalue())


class TestMain(TestCase):
    with temp_dir() as log_dir:
        argv = ['an-env', '/bin/juju', log_dir, 'an-env-mod']
        bs_manager = Mock()
        with patch.object(pxc, 'run_perfscale_test') as mock_run_pt:
            with patch.object(pxc.BootstrapManager, 'from_args',
                              return_value=bs_manager):
                pxc.main(argv)
        mock_run_pt.assert_called_once_with(
            pxc.assess_xplod_perf, bs_manager, _get_default_args(logs=log_dir))
