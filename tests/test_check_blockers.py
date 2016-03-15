from unittest import TestCase
from mock import Mock, patch

import check_blockers


JUJUBOT_USER = {'login': 'jujubot', 'id': 7779494}
OTHER_USER = {'login': 'user', 'id': 1}

SERIES_LIST = {
    'entries': [
        {'name': 'trunk'},
        {'name': '1.20'},
        {'name': '1.21'},
        {'name': '1.22'},
    ]}


def make_fake_lp(series=False, bugs=False):
    """Return a fake Lp lib object based on Mocks"""
    if bugs:
        task_1 = Mock(
            self_link='https://lp/j/98765', title='one', status='Triaged')
        task_2 = Mock(
            self_link='https://lp/j/54321', title='two', status='Triaged')
        bugs = [task_1, task_2]
    else:
        bugs = []
    lp = Mock(_target=None, projects={})
    project = Mock()
    if series:
        series = Mock()
        series.searchTasks.return_value = bugs
        lp._target = series
    else:
        series = None
        project.searchTasks.return_value = bugs
        lp._target = project
    project.getSeries.return_value = series
    lp.projects['juju-core'] = project
    return lp


class CheckBlockers(TestCase):

    def test_parse_args_check(self):
        args = check_blockers.parse_args(['check', 'master', '17'])
        self.assertEqual('check', args.command)
        self.assertEqual('master', args.branch)
        self.assertEqual('17', args.pull_request)

    def test_parse_args_check_pr_optional(self):
        args = check_blockers.parse_args(['check', 'master'])
        self.assertEqual('check', args.command)
        self.assertEqual('master', args.branch)
        self.assertEqual(None, args.pull_request)

    def test_parse_args_check_branch_optional(self):
        args = check_blockers.parse_args(['check'])
        self.assertEqual('check', args.command)
        self.assertEqual('master', args.branch)
        self.assertEqual(None, args.pull_request)

    def test_parse_args_update(self):
        args = check_blockers.parse_args(
            ['-c', './foo.cred', 'update', 'master', '1234'])
        self.assertEqual('update', args.command)
        self.assertEqual('master', args.branch)
        self.assertEqual('1234', args.build)
        self.assertEqual('./foo.cred', args.credentials_file)

    def test_main_check(self):
        bugs = {}
        args = check_blockers.parse_args(['check', 'master', '17'])
        with patch('check_blockers.get_lp', autospec=True,
                   return_value='lp') as gl:
            with patch('check_blockers.get_lp_bugs', autospec=True,
                       return_value=bugs) as glb:
                with patch('check_blockers.get_reason', autospec=True,
                           return_value=(0, 'foo')) as gr:
                    code = check_blockers.main(['check', 'master', '17'])
        gl.assert_called_with('check_blockers', credentials_file=None)
        glb.assert_called_with('lp', 'master', ['blocker'])
        gr.assert_called_with(bugs, args)
        self.assertEqual(0, code)

    def test_main_update(self):
        bugs = {}
        argv = ['-c', './foo.cred', 'update', '--dry-run', 'master', '1234']
        with patch('check_blockers.get_lp', autospec=True,
                   return_value='lp') as gl:
            with patch('check_blockers.get_lp_bugs', autospec=True,
                       return_value=bugs) as glb:
                with patch('check_blockers.update_bugs', autospec=True,
                           return_value=[0, 'Updating']) as ub:
                    code = check_blockers.main(argv)
        gl.assert_called_with('check_blockers', credentials_file='./foo.cred')
        glb.assert_called_with('lp', 'master', ['blocker', 'ci'])
        ub.assert_called_with(bugs, 'master', '1234', dry_run=True)
        self.assertEqual(0, code)

    def test_get_lp_bugs_with_master_branch(self):
        lp = make_fake_lp(series=False, bugs=True)
        bugs = check_blockers.get_lp_bugs(lp, 'master', ['blocker'])
        self.assertEqual(['54321', '98765'], sorted(bugs.keys()))
        project = lp.projects['juju-core']
        self.assertEqual(0, project.getSeries.call_count)
        project.searchTasks.assert_called_with(
            status=check_blockers.BUG_STATUSES,
            importance=check_blockers.BUG_IMPORTANCES,
            tags=check_blockers.BUG_TAGS, tags_combinator='All')

    def test_get_lp_bugs_with_supported_branch(self):
        lp = make_fake_lp(series=True, bugs=True)
        bugs = check_blockers.get_lp_bugs(lp, '1.20', ['blocker'])
        self.assertEqual(['54321', '98765'], sorted(bugs.keys()))
        project = lp.projects['juju-core']
        project.getSeries.assert_called_with(name='1.20')
        series = lp._target
        series.searchTasks.assert_called_with(
            status=check_blockers.BUG_STATUSES,
            importance=check_blockers.BUG_IMPORTANCES,
            tags=check_blockers.BUG_TAGS, tags_combinator='All')

    def test_get_lp_bugs_with_unsupported_branch(self):
        lp = make_fake_lp(series=False, bugs=False)
        bugs = check_blockers.get_lp_bugs(lp, 'foo', ['blocker'])
        self.assertEqual({}, bugs)
        project = lp.projects['juju-core']
        project.getSeries.assert_called_with(name='foo')
        self.assertEqual(0, project.searchTasks.call_count)

    def test_get_lp_bugs_without_blocking_bugs(self):
        lp = make_fake_lp(series=False, bugs=False)
        bugs = check_blockers.get_lp_bugs(lp, 'master', ['blocker'])
        self.assertEqual({}, bugs)
        project = lp.projects['juju-core']
        project.searchTasks.assert_called_with(
            status=check_blockers.BUG_STATUSES,
            importance=check_blockers.BUG_IMPORTANCES,
            tags=check_blockers.BUG_TAGS, tags_combinator='All')

    def test_get_lp_bugs_error(self):
        lp = make_fake_lp(series=False, bugs=True)
        with self.assertRaises(ValueError):
            check_blockers.get_lp_bugs(lp, 'master')

    def test_get_reason_without_blocking_bugs(self):
        args = check_blockers.parse_args(['check', 'master', '17'])
        with patch('check_blockers.get_json') as gj:
            code, reason = check_blockers.get_reason({}, args)
            self.assertEqual(0, code)
            self.assertEqual('No blocking bugs', reason)
            self.assertEqual(0, gj.call_count)

    def test_get_reason_without_comments(self):
        args = check_blockers.parse_args(['check', 'master', '17'])
        with patch('check_blockers.get_json') as gj:
            gj.return_value = []
            bugs = {'98765': {'self_link': 'https://lp/j/98765'}}
            code, reason = check_blockers.get_reason(bugs, args)
            self.assertEqual(1, code)
            self.assertEqual("Does not match ['fixes-98765']", reason)
            gj.assert_called_with((check_blockers.GH_COMMENTS.format('17')))

    def test_get_reason_with_blockers_no_match(self):
        args = check_blockers.parse_args(['check', 'master', '17'])
        with patch('check_blockers.get_json') as gj:
            gj.return_value = [{'body': '$$merge$$', 'user': OTHER_USER}]
            bugs = {'98765': {'self_link': 'https://lp/j/98765'}}
            code, reason = check_blockers.get_reason(bugs, args)
            self.assertEqual(1, code)
            self.assertEqual("Does not match ['fixes-98765']", reason)

    def test_get_reason_with_blockers_with_match(self):
        args = check_blockers.parse_args(['check', 'master', '17'])
        with patch('check_blockers.get_json') as gj:
            gj.return_value = [
                {'body': '$$merge$$', 'user': OTHER_USER},
                {'body': 'la la __fixes-98765__ ha ha', 'user': OTHER_USER}]
            bugs = {'98765': {'self_link': 'https://lp/j/98765'}}
            code, reason = check_blockers.get_reason(bugs, args)
            self.assertEqual(0, code)
            self.assertEqual("Matches fixes-98765", reason)

    def test_get_reason_with_blockers_with_jujubot_comment(self):
        args = check_blockers.parse_args(['check', 'master', '17'])
        with patch('check_blockers.get_json') as gj:
            gj.return_value = [
                {'body': '$$merge$$', 'user': OTHER_USER},
                {'body': 'la la $$fixes-98765$$ ha ha', 'user': JUJUBOT_USER}]
            bugs = {'98765': {'self_link': 'https://lp/j/98765'}}
            code, reason = check_blockers.get_reason(bugs, args)
            self.assertEqual(1, code)
            self.assertEqual("Does not match ['fixes-98765']", reason)

    def test_get_reason_with_blockers_with_reply_jujubot_comment(self):
        args = check_blockers.parse_args(['check', 'master', '17'])
        with patch('check_blockers.get_json') as gj:
            gj.return_value = [
                {'body': '$$merge$$', 'user': OTHER_USER},
                {'body': 'Juju bot wrote $$fixes-98765$$', 'user': OTHER_USER}]
            bugs = {'98765': {'self_link': 'https://lp/j/98765'}}
            code, reason = check_blockers.get_reason(bugs, args)
            self.assertEqual(1, code)
            self.assertEqual("Does not match ['fixes-98765']", reason)

    def test_get_reason_with_blockers_with_jfdi(self):
        args = check_blockers.parse_args(['check', 'master', '17'])
        with patch('check_blockers.get_json') as gj:
            gj.return_value = [
                {'body': '$$merge$$', 'user': OTHER_USER},
                {'body': 'la la __JFDI__ ha ha', 'user': OTHER_USER}]
            bugs = {'98765': {'self_link': 'https://lp/j/98765'}}
            code, reason = check_blockers.get_reason(bugs, args)
            self.assertEqual(0, code)
            self.assertEqual("Engineer says JFDI", reason)

    def test_get_json(self):
        response = Mock()
        response.getcode.return_value = 200
        response.read.side_effect = ['{"result": []}']
        with patch('check_blockers.urllib2.urlopen') as urlopen:
            urlopen.return_value = response
            json = check_blockers.get_json("http://api.testing/")
            request = urlopen.call_args[0][0]
            self.assertEqual(request.get_full_url(), "http://api.testing/")
            self.assertEqual(request.get_header("Cache-control"),
                             "max-age=0, must-revalidate")
            self.assertEqual(json, {"result": []})

    def test_update_bugs(self):
        lp = make_fake_lp(series=False, bugs=True)
        bugs = check_blockers.get_lp_bugs(lp, 'master', ['blocker'])
        code, changes = check_blockers.update_bugs(
            bugs, 'master', '1234', dry_run=False)
        self.assertEqual(0, code)
        self.assertIn('Updated two', changes)
        self.assertEqual('Fix Released', bugs['54321'].status)
        self.assertEqual(1, bugs['54321'].lp_save.call_count)
        expected_subject = 'Fix Released in juju-core master'
        expected_content = (
            'Juju-CI verified that this issue is %s:\n'
            '    http://reports.vapour.ws/releases/1234' % expected_subject)
        bugs['54321'].bug.newMessage.assert_called_with(
            subject=expected_subject, content=expected_content)

    def test_update_bugs_with_dry_run(self):
        lp = make_fake_lp(series=False, bugs=True)
        bugs = check_blockers.get_lp_bugs(lp, 'master', ['blocker'])
        code, changes = check_blockers.update_bugs(
            bugs, 'master', '1234', dry_run=True)
        self.assertEqual(0, bugs['54321'].lp_save.call_count)
