"""Tests for assess_perf_test_simple module."""

import argparse
from mock import patch, Mock, call
import StringIO

from jujupy import fake_juju_client
import perfscale_longrunning as pl
from tests import (
    parse_error,
    TestCase,
)


def get_default_args(log_dir='/tmp/logs', run_length=12):
    return argparse.Namespace(
        env='an-env',
        juju_bin='/bin/juju',
        logs=log_dir,
        temp_env_name='an-env-mod',
        run_length=run_length,
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
        args = pl.parse_args(
            [
                'an-env',
                '/bin/juju',
                '/tmp/logs',
                'an-env-mod'])
        self.assertEqual(
            args,
            get_default_args()
        )

    def test_default_run_length(self):
        args = pl.parse_args(
            ['an-env', '/bin/juju', '/tmp/logs', 'an-env-mod'])
        self.assertEqual(args.run_length, 12)

    def test_passing_bundle_name(self):
        args = pl.parse_args(
            [
                'an-env',
                '/bin/juju',
                '/tmp/logs',
                'an-env-mod',
                '--run-length', '42'])
        self.assertEqual(args.run_length, 42)

    def test_help(self):
        fake_stdout = StringIO.StringIO()
        with parse_error(self) as fake_stderr:
            with patch('sys.stdout', fake_stdout):
                pl.parse_args(['--help'])
        self.assertEqual('', fake_stderr.getvalue())
        self.assertIn(
            'Perfscale longrunning test.', fake_stdout.getvalue())


class TestActionCleanup(TestCase):

    def test_destroy_all_models(self):
        client = Mock()
        new_models = [Mock(), Mock()]

        pl.action_cleanup(client, new_models)

        client.destroy_model.assert_called_once_with()
        new_models[0].destroy_model.assert_called_once_with()
        new_models[1].destroy_modelassert_called_once_with()


class TestActionBusy(TestCase):

    def test_must_add_multiple_models(self):
        client = fake_juju_client()
        client.bootstrap()
        with patch.object(pl.time, 'sleep', autospec=True):
            with patch.object(pl, 'until_timeout', autospec=True):
                new_models = pl.action_busy(client, [])
                created_models = [
                    m[1] for m
                    in client._backend.controller_state.models.items()
                    if m[1].name not in ('name', 'controller')]
                self.assertEqual(len(new_models), 20)
                self.assertEqual(len(created_models), 20)

    def test_must_add_units_to_named_applications(self):
        client = Mock()
        client.add_model.return_value = Mock()

        with patch.object(pl.time, 'sleep', autospec=True):
            with patch.object(pl, 'until_timeout', autospec=True):

                pl.action_busy(client, ['app1', 'app2'])

                self.assertEqual(
                    client.juju.call_args_list,
                    [
                        call('add-unit', ('app1', '-n', '1')),
                        call('add-unit', ('app2', '-n', '1')),
                    ])


class TestActionCreate(TestCase):

    def test_deploys_stack_to_new_model(self):
        client = Mock()
        new_model = Mock()
        client.add_model.return_value = new_model

        with patch.object(pl, 'deploy_stack', autospec=True) as m_deploy_stack:
            pl.action_create(client, series='foo')

        m_deploy_stack.assert_called_once_with(new_model, 'foo')
        client.add_model.assert_called_once_with('newmodel')


class TestPerfscaleLongrunPerf(TestCase):

    def test_must_use_test_length_in_seconds(self):
        client = Mock()
        pprof_collector = Mock()
        args = argparse.Namespace(run_length=1)
        with patch.object(pl, 'until_timeout', autospec=True) as m_ut:
            pl.perfscale_longrun_perf(client, pprof_collector, args)
        m_ut.assert_called_once_with(3600)

    def test_perfscale_longrun_perf(self):
        client = Mock()
        pprof_collector = Mock()
        new_client = Mock()
        new_models = [Mock(), Mock()]
        args = argparse.Namespace(run_length=1)

        with patch.object(
                pl, 'until_timeout',
                autospec=True, return_value=[1]):
            with patch.object(pl, 'action_rest', autospec=True):
                with patch.object(
                        pl, 'action_create',
                        autospec=True, return_value=new_client) as m_ac:
                    with patch.object(
                            pl, 'action_busy',
                            autospec=True, return_value=new_models) as m_ab:
                            with patch.object(
                                    pl, 'action_cleanup',
                                    autospec=True) as m_acu:
                                pl.perfscale_longrun_perf(
                                    client, pprof_collector, args)

        m_ac.assert_called_once_with(client)
        m_ab.assert_called_once_with(new_client, ['dummy-sink'])
        m_acu.assert_called_once_with(new_client, new_models)
