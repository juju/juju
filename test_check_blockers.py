from unittest import TestCase
from mock import patch

import check_blockers


JUJUBOT_USER = {'login': 'jujubot', 'id': 7779494}
OTHER_USER = {'login': 'user', 'id': 1}


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
        with patch('check_blockers.DEVEL') as devel:
            devel.return_value = '1.20'
            with patch('check_blockers.get_json') as gj:
                data = {'entries': []}
                gj.return_value = data
                check_blockers.get_lp_bugs(args)
                gj.assert_called_with(
                    (check_blockers.LP_BUGS.format('juju-core/1.20')))

    def test_get_lp_bugs_without_blocking_bugs(self):
        args = check_blockers.parse_args(['master', '17'])
        with patch('check_blockers.get_json') as gj:
            empty_bug_list = {'entries': []}
            gj.return_value = empty_bug_list
            bugs = check_blockers.get_lp_bugs(args)
            self.assertEqual({}, bugs)

    def test_get_lp_bugs_with_blocking_bugs(self):
        args = check_blockers.parse_args(['master', '17'])
        with patch('check_blockers.get_json') as gj:
            bug_list = {
                'entries': [
                    {'self_link': 'https://lp/j/98765'},
                    {'self_link': 'https://lp/j/54321'},
                    ]}
            gj.return_value = bug_list
            bugs = check_blockers.get_lp_bugs(args)
            self.assertEqual(['54321', '98765'], sorted(bugs.keys()))

    def test_get_reason_without_blocking_bugs(self):
        args = check_blockers.parse_args(['master', '17'])
        with patch('check_blockers.get_json') as gj:
            code, reason = check_blockers.get_reason({}, args)
            self.assertEqual(0, code)
            self.assertEqual('No blocking bugs', reason)
            self.assertEqual(0, gj.call_count)

    def test_get_reason_without_comments(self):
        args = check_blockers.parse_args(['master', '17'])
        with patch('check_blockers.get_json') as gj:
            gj.return_value = []
            bugs = {'98765': {'self_link': 'https://lp/j/98765'}}
            code, reason = check_blockers.get_reason(bugs, args)
            self.assertEqual(1, code)
            self.assertEqual('Could not get 17 comments from github', reason)
            gj.assert_called_with((check_blockers.GH_COMMENTS.format('17')))

    def test_get_reason_with_blockers_no_match(self):
        args = check_blockers.parse_args(['master', '17'])
        with patch('check_blockers.get_json') as gj:
            gj.return_value = [{'body': '$$merge$$', 'user': OTHER_USER}]
            bugs = {'98765': {'self_link': 'https://lp/j/98765'}}
            code, reason = check_blockers.get_reason(bugs, args)
            self.assertEqual(1, code)
            self.assertEqual("Does not match ['$$fixes-98765$$']", reason)

    def test_get_reason_with_blockers_with_match(self):
        args = check_blockers.parse_args(['master', '17'])
        with patch('check_blockers.get_json') as gj:
            gj.return_value = [
                {'body': '$$merge$$', 'user': OTHER_USER},
                {'body': 'la la $$fixes-98765$$ ha ha', 'user': OTHER_USER}]
            bugs = {'98765': {'self_link': 'https://lp/j/98765'}}
            code, reason = check_blockers.get_reason(bugs, args)
            self.assertEqual(0, code)
            self.assertEqual("Matches $$fixes-98765$$", reason)

    def test_get_reason_with_blockers_with_jujubot_comment(self):
        args = check_blockers.parse_args(['master', '17'])
        with patch('check_blockers.get_json') as gj:
            gj.return_value = [
                {'body': '$$merge$$', 'user': OTHER_USER},
                {'body': 'la la $$fixes-98765$$ ha ha', 'user': JUJUBOT_USER}]
            bugs = {'98765': {'self_link': 'https://lp/j/98765'}}
            code, reason = check_blockers.get_reason(bugs, args)
            self.assertEqual(1, code)
            self.assertEqual("Does not match ['$$fixes-98765$$']", reason)

    def test_get_reason_with_blockers_with_reply_jujubot_comment(self):
        args = check_blockers.parse_args(['master', '17'])
        with patch('check_blockers.get_json') as gj:
            gj.return_value = [
                {'body': '$$merge$$', 'user': OTHER_USER},
                {'body': 'Juju bot wrote $$fixes-98765$$', 'user': OTHER_USER}]
            bugs = {'98765': {'self_link': 'https://lp/j/98765'}}
            code, reason = check_blockers.get_reason(bugs, args)
            self.assertEqual(1, code)
            self.assertEqual("Does not match ['$$fixes-98765$$']", reason)
