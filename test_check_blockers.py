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
        task_1 = Mock(self_link='https://lp/j/98765')
        task_2 = Mock(self_link='https://lp/j/54321')
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
        with patch('check_blockers.get_lp_bugs', autospec=True,
                   return_value=bugs) as glb:
            with patch('check_blockers.get_reason', autospec=True,
                       return_value=(0, 'foo')) as gr:
                code = check_blockers.main(['check', 'master', '17'])
        glb.assert_called_with(args, with_ci=False)
        gr.assert_called_with(bugs, args)
        self.assertEqual(0, code)

    def test_main_update(self):
        bugs = {}
        argv = ['-c', './foo.cred', 'update', '--dry-run', 'master', '1234']
        args = check_blockers.parse_args(argv)
        with patch('check_blockers.get_lp_bugs', autospec=True,
                   return_value=bugs) as glb:
            with patch('check_blockers.update_bugs', autospec=True,
                       return_value=[0, 'Updating']) as ub:
                code = check_blockers.main(argv)
        glb.assert_called_with(args, with_ci=True)
        ub.assert_called_with(bugs, dry_run=True)
        self.assertEqual(0, code)

    def test_get_lp_bugs_with_master_branch(self):
        args = check_blockers.parse_args(['check', 'master', '17'])
        lp = make_fake_lp(series=False, bugs=True)
        with patch('check_blockers.get_lp', autospec=True,
                   return_value=lp) as gl:
            bugs = check_blockers.get_lp_bugs(args)
        self.assertEqual(['54321', '98765'], sorted(bugs.keys()))
        gl.assert_called_with('check_blockers', credentials_file=None)
        project = lp.projects['juju-core']
        self.assertEqual(0, project.getSeries.call_count)
        project.searchTasks.assert_called_with(
            status=check_blockers.BUG_STATUSES,
            importance=check_blockers.BUG_IMPORTANCES,
            tags=check_blockers.BUG_TAGS, tags_combinator='All')

    def test_get_lp_bugs_with_supported_branch(self):
        args = check_blockers.parse_args(['check', '1.20', '17'])
        lp = make_fake_lp(series=True, bugs=True)
        with patch('check_blockers.get_lp', autospec=True,
                   return_value=lp) as gl:
            bugs = check_blockers.get_lp_bugs(args)
        self.assertEqual(['54321', '98765'], sorted(bugs.keys()))
        gl.assert_called_with('check_blockers', credentials_file=None)
        project = lp.projects['juju-core']
        project.getSeries.assert_called_with(name='1.20')
        series = lp._target
        series.searchTasks.assert_called_with(
            status=check_blockers.BUG_STATUSES,
            importance=check_blockers.BUG_IMPORTANCES,
            tags=check_blockers.BUG_TAGS, tags_combinator='All')

    def test_get_lp_bugs_with_unsupported_branch(self):
        args = check_blockers.parse_args(['check', 'foo', '17'])
        lp = make_fake_lp(series=False, bugs=False)
        with patch('check_blockers.get_lp', autospec=True, return_value=lp):
            bugs = check_blockers.get_lp_bugs(args)
        self.assertEqual({}, bugs)
        project = lp.projects['juju-core']
        project.getSeries.assert_called_with(name='foo')
        self.assertEqual(0, project.searchTasks.call_count)

    def test_get_lp_bugs_without_blocking_bugs(self):
        args = check_blockers.parse_args(['check', 'master', '17'])
        lp = make_fake_lp(series=False, bugs=False)
        with patch('check_blockers.get_lp', autospec=True, return_value=lp):
            bugs = check_blockers.get_lp_bugs(args)
        self.assertEqual({}, bugs)
        project = lp.projects['juju-core']
        project.searchTasks.assert_called_with(
            status=check_blockers.BUG_STATUSES,
            importance=check_blockers.BUG_IMPORTANCES,
            tags=check_blockers.BUG_TAGS, tags_combinator='All')

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
            self.assertEqual('Could not get 17 comments from github', reason)
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
        response.read.side_effect = ['{"result": []}']
        with patch('check_blockers.urllib2.urlopen') as urlopen:
            urlopen.return_value = response
            json = check_blockers.get_json("http://api.testing/")
            request = urlopen.call_args[0][0]
            self.assertEqual(request.get_full_url(), "http://api.testing/")
            self.assertEqual(request.get_header("Cache-control"),
                             "max-age=0, must-revalidate")
            self.assertEqual(json, {"result": []})
