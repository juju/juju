from unittest import TestCase
from mock import patch

import check_blockers


class CheckBlockers(TestCase):

    def test_parse_args(self):
        args = check_blockers.parse_args(['master', '17'])
        self.assertEqual('master', args.branch)
        self.assertEqual('17', args.pull_request)

    def test_get_lp_bugs_with_master(self):
        args = check_blockers.parse_args(['master', '17'])
        with patch('check_blockers.get_json') as gj:
            data = {'entries': []}
            gj.return_value = data
            check_blockers.get_lp_bugs(args)
            gj.assert_called_with((check_blockers.LP_BUGS.format('juju-core')))

    def test_get_lp_bugs_with_devel(self):
        args = check_blockers.parse_args(['1.20', '17'])
        with patch('check_blockers.get_json') as gj:
            data = {'entries': []}
            gj.return_value = data
            check_blockers.get_lp_bugs(args)
            gj.assert_called_with(
                (check_blockers.LP_BUGS.format('juju-core/1.20')))

    def test_get_lp_bugs_no_blocking_bugs(self):
        args = check_blockers.parse_args(['master', '17'])
        with patch('check_blockers.get_json') as gj:
            empty_bug_list = {'entries': []}
            gj.return_value = empty_bug_list
            bugs = check_blockers.get_lp_bugs(args)
            self.assertEqual({}, bugs)
