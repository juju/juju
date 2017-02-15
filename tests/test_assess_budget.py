"""Tests for assess_budget module."""

import logging
import StringIO
from random import randint

from mock import (
    Mock,
    patch,
    )

from assess_budget import (
    assess_budget,
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
            autospec=True) as sub_mock:
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
        self.budget_name = 'test'
        self.budget_limit = randint(1000,10000)
        self.budget_value = str(randint(100,900))

    def test_assess_budget(self):
        fake_client = Mock(wraps=fake_juju_client())
          
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
                        assess_budget(fake_client)
                        
                        init_b_mock.assert_called_once_with(fake_client,
                            self.budget_name, '0')
                        expect_b_mock.assert_called_once_with(
                            update_e_mock(fake_client),
                            self.budget_name, self.budget_value)
                        b_limit_mock.assert_called_once_with(fake_client, 0)
                        create_b_mock.assert_called_once_with(fake_client,
                            self.budget_name, self.budget_value, 0)
                        set_b_mock.assert_called_once_with(fake_client,
                            self.budget_name, self.budget_value, 0)
                        show_b_mock.assert_called_once_with(fake_client,
                            self.budget_name, self.budget_value)
                        list_b_mock.assert_called_once_with(fake_client,
                            update_e_mock(fake_client))
                            
                        self.assertEqual(init_b_mock.call_count, 1)
                        self.assertEqual(expect_b_mock.call_count, 1)
                        self.assertEqual(b_limit_mock.call_count, 1)
                        self.assertEqual(create_b_mock.call_count, 1)
                        self.assertEqual(set_b_mock.call_count, 1)
                        self.assertEqual(show_b_mock.call_count, 1)
                        self.assertEqual(list_b_mock.call_count, 1)


    def test_assess_show_budget(self):
        fake_client = fake_juju_client()
        with patch.object(fake_client, 'get_juju_output'):
            with patch("assess_budget.json.loads"):
                with self.assertRaises(JujuAssertionError):
                    assess_show_budget(fake_client, self.budget_name,
                        self.budget_value)

    #def test_assess_list_budgets(self):

    #def test_assess_set_budget(self):

    #def test_assess_create_budget(self):        

    #def test_assess_budget_limt(self):
