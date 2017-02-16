"""Tests for assess_budget module."""

import logging
import json
from random import randint
import StringIO
from subprocess import CalledProcessError

from mock import (
    Mock,
    patch,
)

from assess_budget import (
    _try_greater_than_limit_budget,
    _try_negative_budget,
    assess_budget,
    assess_budget_limit,
    assess_create_budget,
    assess_list_budgets,
    assess_set_budget,
    assess_show_budget,
    main,
    parse_args,
)
from jujupy import (
    fake_juju_client,
)
from tests import (
    parse_error,
    TestCase,
)
from utility import (
    JujuAssertionError,
)


class TestParseArgs(TestCase):

    def test_common_args(self):
        args = parse_args(["an-env", "/bin/juju", "/tmp/logs", "an-env-mod"])
        self.assertEqual("an-env", args.env)
        self.assertEqual("/bin/juju", args.juju_bin)
        self.assertEqual("/tmp/logs", args.logs)
        self.assertEqual("an-env-mod", args.temp_env_name)
        self.assertEqual(False, args.debug)

    def test_help(self):
        fake_stdout = StringIO.StringIO()
        with parse_error(self) as fake_stderr:
            with patch("sys.stdout", fake_stdout):
                parse_args(["--help"])
        self.assertEqual("", fake_stderr.getvalue())


class TestMain(TestCase):

    def test_main(self):
        argv = ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod", "--verbose"]
        client = Mock(spec=["is_jes_enabled"])
        with patch('assess_budget.subprocess.check_output',
                   autospec=True):
            with patch("assess_budget.configure_logging",
                       autospec=True) as mock_cl:
                with patch('assess_budget.client_from_config',
                           return_value=client) as mock_cfc:
                    with patch("assess_budget.assess_budget",
                               autospec=True) as mock_assess:
                        main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_cfc.assert_called_once_with('an-env', "/bin/juju", False)
        mock_assess.assert_called_once_with(client)


class TestAssess(TestCase):

    def setUp(self):
        super(TestAssess, self).setUp()
        self.fake_client = fake_juju_client()
        self.budget_name = 'test'
        self.budget_limit = randint(1000, 10000)
        self.budget_value = str(randint(100, 900))

    def test_assess_budget(self):
        show_b = patch("assess_budget.assess_show_budget",
                       autospec=True)
        list_b = patch("assess_budget.assess_list_budgets",
                       autospec=True)
        set_b = patch("assess_budget.assess_set_budget",
                      autospec=True)
        create_b = patch("assess_budget.assess_create_budget",
                         autospec=True)
        b_limit = patch("assess_budget.assess_budget_limit",
                        autospec=True)
        init_b = patch("assess_budget._try_setting_budget",
                       autospec=True)
        expect_b = patch("assess_budget._set_budget_value_expectations",
                         autospec=True)
        update_e = patch("assess_budget._get_budgets", autospec=True)

        with show_b as show_b_mock, list_b as list_b_mock, \
            set_b as set_b_mock, create_b as create_b_mock, \
            b_limit as b_limit_mock, init_b as init_b_mock, \
                expect_b as expect_b_mock, update_e as update_e_mock:
                with patch("assess_budget.json.loads"):
                    with patch("assess_budget.randint",
                               return_value=self.budget_value):
                        assess_budget(self.fake_client)

                        init_b_mock.assert_called_once_with(self.fake_client,
                                                            self.budget_name,
                                                            '0')
                        expect_b_mock.assert_called_once_with(
                            update_e_mock(self.fake_client),
                            self.budget_name, self.budget_value)
                        b_limit_mock.assert_called_once_with(0)
                        create_b_mock.assert_called_once_with(
                            self.fake_client, self.budget_name,
                            self.budget_value, 0)
                        set_b_mock.assert_called_once_with(
                            self.fake_client, self.budget_name,
                            self.budget_value, 0)
                        show_b_mock.assert_called_once_with(
                            self.fake_client, self.budget_name,
                            self.budget_value)
                        list_b_mock.assert_called_once_with(
                            self.fake_client, update_e_mock(self.fake_client))

                        self.assertEqual(init_b_mock.call_count, 1)
                        self.assertEqual(expect_b_mock.call_count, 1)
                        self.assertEqual(b_limit_mock.call_count, 1)
                        self.assertEqual(create_b_mock.call_count, 1)
                        self.assertEqual(set_b_mock.call_count, 1)
                        self.assertEqual(show_b_mock.call_count, 1)
                        self.assertEqual(list_b_mock.call_count, 1)


class TestAssessShowBudget(TestAssess):

    def setUp(self):
        super(TestAssessShowBudget, self).setUp()
        self.fake_json = json.loads('{"limit":"' + self.budget_value +
                                    '","total":{"usage":"0%"}}')

    def test_assess_show_budget(self):
        with patch.object(self.fake_client, 'get_juju_output'):
            with patch("assess_budget.json.loads",
                       return_value=self.fake_json):
                    assess_show_budget(self.fake_client, self.budget_name,
                                       self.budget_value)

    def test_raises_budget_usage_error(self):
        error_usage = randint(1, 100)
        self.fake_json['total']['usage'] = error_usage
        with patch.object(self.fake_client, 'get_juju_output'):
            with patch("assess_budget.json.loads",
                       return_value=self.fake_json):
                with self.assertRaises(JujuAssertionError) as ex:
                    assess_show_budget(self.fake_client, self.budget_name,
                                       self.budget_value)
                self.assertEqual(ex.exception.message,
                                 'Budget usage found {}, expected 0%'.format(
                                    error_usage))

    def test_raises_budget_limit_error(self):
        self.fake_json['limit'] = 0
        with patch.object(self.fake_client, 'get_juju_output'):
            with patch("assess_budget.json.loads",
                       return_value=self.fake_json):
                with self.assertRaises(JujuAssertionError) as ex:
                    assess_show_budget(self.fake_client, self.budget_name,
                                       self.budget_value)
                self.assertEqual(ex.exception.message,
                                 'Budget limit found 0, expected {}'.format(
                                     self.budget_value))


class TestAssessListBudgets(TestAssess):

    def setUp(self):
        super(TestAssessListBudgets, self).setUp()
        snippet = '[{"budget": "test", "limit": "300"}]'
        self.fake_budgets_json = json.loads('{"budgets":' + snippet + '}')
        self.fake_budget_json = json.loads(snippet)
        unexpected_snippet = '[{"budget": "test", "limit": "100"}]'
        self.fake_unexpected_budgets_json = json.loads(
            '{"budgets":' + unexpected_snippet + '}')
        self.fake_unexpected_budget_json = json.loads(unexpected_snippet)

    def test_assess_list_budgets(self):
        with patch.object(self.fake_client, 'get_juju_output'):
            with patch("assess_budget.json.loads",
                       return_value=self.fake_budgets_json):
                assess_list_budgets(self.fake_client, self.fake_budget_json)

    def test_raises_list_mismatch(self):
        with patch.object(self.fake_client, 'get_juju_output'):
            with patch("assess_budget.json.loads",
                       return_value=self.fake_unexpected_budgets_json):
                with self.assertRaises(JujuAssertionError) as ex:
                    assess_list_budgets(
                        self.fake_client, self.fake_budget_json)
                self.assertEqual(ex.exception.message,
                                 'Found: {}\nExpected: {}'.format(
                                     self.fake_unexpected_budget_json,
                                     self.fake_budget_json))


class TestAssessSetBudget(TestAssess):

    def test_assess_set_budget(self):
        with patch.object(self.fake_client, 'get_juju_output'):
            with patch("assess_budget.json.loads"):
                with patch("assess_budget._try_setting_budget"):
                    with patch("assess_budget.assert_set_budget"):
                        assess_set_budget(self.fake_client, self.budget_name,
                                          self.budget_value, self.budget_limit)

    def test_raises_on_exceed_credit_limit(self):
        with patch.object(self.fake_client, 'get_juju_output'):
            with patch("assess_budget.json.loads"):
                with patch("assess_budget._try_setting_budget"):
                    with self.assertRaises(JujuAssertionError) as ex:
                        _try_greater_than_limit_budget(self.fake_client,
                                                       self.budget_name,
                                                       self.budget_limit)
                    self.assertEqual(ex.exception.message,
                                     'Credit limit exceeded')

    def test_raises_on_negative_budget(self):
        self.budget_limit = -abs(self.budget_limit)
        with patch.object(self.fake_client, 'get_juju_output'):
            with patch("assess_budget.json.loads"):
                with patch("assess_budget._try_setting_budget"):
                    with self.assertRaises(JujuAssertionError) as ex:
                        _try_negative_budget(self.fake_client,
                                             self.budget_name)
                    self.assertEqual(ex.exception.message,
                                     'Negative budget allowed')


class TestAssessCreateBudget(TestAssess):

    def test_assess_create_budget(self):
        with patch.object(self.fake_client, 'get_juju_output'):
            with patch("assess_budget.create_budget"):
                assess_create_budget(self.fake_client, self.budget_name,
                                     self.budget_value, self.budget_limit)

    def test_raises_duplicate_budget(self):
        with patch.object(self.fake_client, 'get_juju_output'):
            with patch("assess_budget.create_budget",
                       side_effect=JujuAssertionError):
                with self.assertRaises(JujuAssertionError) as ex:
                    assess_create_budget(self.fake_client, self.budget_name,
                                         self.budget_value, self.budget_limit)
                self.assertEqual(ex.exception.message,
                                 'Added duplicate budget')

    def test_raises_creation_error(self):
        with patch.object(self.fake_client, 'get_juju_output'):
            with patch("assess_budget.create_budget",
                       side_effect=CalledProcessError(1, 'foo', 'bar')):
                with self.assertRaises(JujuAssertionError) as ex:
                    assess_create_budget(self.fake_client, self.budget_name,
                                         self.budget_value, self.budget_limit)
                self.assertEqual(ex.exception.message,
                                 'Error testing create-budget: bar')


class TestAssessBudgetLimit(TestAssess):

    def test_assess_budget_limt(self):
        budget_limit = randint(1, 10000)
        assess_budget_limit(budget_limit)

    def test_raises_error_on_negative_limit(self):
        neg_budget_limit = randint(-1000, -1)
        with self.assertRaises(JujuAssertionError) as ex:
            assess_budget_limit(neg_budget_limit)
        self.assertEqual(ex.exception.message,
                         'Negative Budget Limit {}'.format(neg_budget_limit))
