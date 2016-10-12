"""Tests for assess_perf_test_simple module."""

from contextlib import contextmanager
from datetime import datetime
from mock import call, patch, Mock
from textwrap import dedent

from fakejuju import fake_juju_client
import generate_perfscale_results as gpr
import perf_graphing
from test_perfscale_deployment import default_args
from test_quickstart_deploy import make_bootstrap_manager
from tests import TestCase


class TestRunPerfscaleTest(TestCase):

    def test_calls_provided_test(self):
        client = fake_juju_client()
        bs_manager = make_bootstrap_manager(client)

        timing = gpr.TimingData(datetime.utcnow(), datetime.utcnow())
        deploy_details = gpr.DeployDetails('test', dict(), timing)
        noop_test = Mock(return_value=deploy_details)

        with patch.object(gpr, 'dump_performance_metrics_logs', autospec=True):
            with patch.object(gpr, 'generate_reports', autospec=True):
                gpr.run_perfscale_test(noop_test, bs_manager, default_args)

        noop_test.assert_called_once_with(client, default_args)


class TestDumpPerformanceMetricsLogs(TestCase):

    def test_pulls_rrd_and_mongostats_logs(self):
        client = Mock()
        res_dir = '/foo/performance_results/'

        with patch.object(gpr.os, 'makedirs', autospec=True) as m_makedirs:
            self.assertEqual(
                res_dir,
                gpr.dump_performance_metrics_logs('/foo', client))
        m_makedirs.assert_called_once_with(res_dir)
        expected_calls = [
            call(
                'scp',
                ('--', '-r', '0:/var/lib/collectd/rrd/localhost/*', res_dir)),
            call(
                'scp',
                ('0:/tmp/mongodb-stats.log', res_dir)
                )]
        self.assertEqual(
            client.juju.call_args_list,
            expected_calls)


class TestGenerateGraphImages(TestCase):

    @contextmanager
    def patch_image_creation_ctx(self, image):
        with patch.object(
                gpr, 'generate_graph_image',
                return_value=image, autospec=True) as m_ggi:
            yield m_ggi

    def test_generate_graph_image(self):
        image = Mock()
        base_dir = '/foo/test'
        results_dir = 'results'
        name = 'testing_name'
        generator = Mock()
        with patch.object(
                gpr, 'create_report_graph', return_value=image) as m_crg:
            self.assertEqual(
                image,
                gpr.generate_graph_image(
                    base_dir, results_dir, name, generator))
        m_crg.assert_called_once_with(
            '/foo/test/results', base_dir, name, generator)

    def test_generate_cpu_graph(self):
        image = Mock()
        with self.patch_image_creation_ctx(image) as m_ggi:
            self.assertEqual(
                image,
                gpr.generate_cpu_graph_image('/foo'))
        m_ggi.assert_called_once_with(
            '/foo', 'aggregation-cpu-average', 'cpu', perf_graphing.cpu_graph)

    def test_generate_memory_graph_calls_(self):
        image = Mock()
        with self.patch_image_creation_ctx(image) as m_ggi:
            self.assertEqual(
                image,
                gpr.generate_memory_graph_image('/foo'))
        m_ggi.assert_called_once_with(
            '/foo', 'memory', 'memory', perf_graphing.memory_graph)

    def test_generate_network_graph(self):
        image = Mock()
        with self.patch_image_creation_ctx(image) as m_ggi:
            self.assertEqual(
                image,
                gpr.generate_network_graph_image('/foo'))
        m_ggi.assert_called_once_with(
            '/foo', 'interface-eth0', 'network', perf_graphing.network_graph)

    def test_generate_mongo_query_graph(self):
        image = Mock()
        with self.patch_image_creation_ctx(image) as m_ggi:
            self.assertEqual(
                image,
                gpr.generate_mongo_query_graph_image('/foo'))
        m_ggi.assert_called_once_with(
            '/foo', 'mongodb', 'mongodb', perf_graphing.mongodb_graph)

    def test_generate_mongo_memory_graph(self):
        image = Mock()
        with self.patch_image_creation_ctx(image) as m_ggi:
            self.assertEqual(
                image,
                gpr.generate_mongo_memory_graph_image('/foo'))
        m_ggi.assert_called_once_with(
            '/foo',
            'mongodb',
            'mongodb_memory',
            perf_graphing.mongodb_memory_graph)


class TestFindActualStart(TestCase):
    example_output = dedent("""\
                         value

    1468551204: -nan
    1468554270: -nan
    1468554273: -nan
    1468554270: -nan
    1468554273: -nan
    1468554276: 1.7516817067e+08
    1468554279: 1.7500023467e+08
    1468554282: 1.7661269333e+08
    1468554285: 1.7819374933e+08""")

    example_multivalue_output = dedent("""\
                             value1    value2

    1472708601: -nan -nan
    1472708604: -nan -nan
    1472708607: -nan -nan
    1472708610: -nan -nan
    1472708613: 7.5466666667e+02 5.8166666667e+02
    1472708616: 2.5555555556e+02 1.9833333333e+02
    1472708619: 1.3333333333e+01 1.1555555556e+01
    1472708622: 2.7444444444e+01 2.6222222222e+01""")

    def test_doesnt_choke_on_non_timestamp_lines(self):
        try:
            gpr.find_actual_start(self.example_output)
            gpr.find_actual_start(self.example_multivalue_output)
        except Exception:
            self.fail('Unexpected exception raised.')

    def test_returns_actual_start_timestamp(self):
        self.assertEqual(
            gpr.find_actual_start(self.example_output),
            '1468554276')

        self.assertEqual(
            gpr.find_actual_start(self.example_multivalue_output),
            '1472708613')
