#!/usr/bin/python3 -u
"""expirebugs.py <project> [options]

Categorize and expire old Launchpad bugs.

This script categorizes a Launchpad project's bugs based on the time they were
last updated and produces a report. If --update is specified, it performs an
"expiration action" on each bug. The categories and actions are as follows:

| Category | Filter                                             | Action
| -------- | -------------------------------------------------- | -----------------------
| Ancient  | last updated >5y ago                               | set status to 'Expired'
| Old      | else last updated >2y ago and not 'Low'/'Wishlist' | set importance to 'Low'
| Aging    | else last updated >60d ago and importance 'Medium' | set importance to 'Low'
| High     | importance 'High' or 'Critical'                    | no action
| Other    | everything else                                    | no action

When the script updates a bug, it also adds an 'expirebugs-bot' tag. If a bug
has this tag, the last-updated date is computed based not on the Launchpad
bug's date_last_updated time (because that would be recent due to the script
action), but based on the time of the last message or activity excluding the
user this script is being run as.

Additionally, when the script updates a bug, it adds a comment (message) along
the lines of "This bug has not been updated in 5 years, so we're marking it
Expired. ..."

The run the script, install the launchpadlib library (documentation at
https://help.launchpad.net/API/launchpadlib) and execute this script file.

Be sure to run it as "juju-qa-bot". It should prompt you for credentials the
first time -- these are saved in your Ubuntu keyring ("Passwords and Keys").
"""

import argparse
import datetime
import sys
import time

from launchpadlib.launchpad import Launchpad

BOT_TAG = 'expirebugs-bot'


def main():
	parser = argparse.ArgumentParser(usage=__doc__)
	parser.add_argument('project', help='project to search for bug tasks, for example "juju"',
		                type=str)
	parser.add_argument('-n', help='stop after n bug tasks (default all)', type=int)
	parser.add_argument('--service-root', help='root URL/name of Launchpad instance (default %(default)r)',
		                type=str, default='qastaging')
	parser.add_argument('--show-progress', help='show progress dot for every bug processed', action='store_true')
	parser.add_argument('--update', help='actually update bugs in Launchpad', action='store_true')
	parser.add_argument('--verbose', help='verbose: show individual bugs in each category', action='store_true')
	args = parser.parse_args()

	launchpad = Launchpad.login_with('expirebugs', service_root=args.service_root, version='devel')
	self_link = launchpad.me.self_link
	print(f'Running as (self-link): {launchpad.me.web_link}', file=sys.stderr)

	start = time.time()
	num_bugs = 0
	ancients = []
	olds = []
	aging = []
	high = []
	other = []
	last_updated_map = {}
	send_notifications = 'staging' not in args.service_root

	now = datetime.datetime.now(tz=datetime.timezone.utc)

	project = launchpad.projects[args.project]
	tasks = project.searchTasks(status=[
	    'New',
	    'Incomplete',
	    'Confirmed',
	    'Triaged',
	    'In Progress',
	    'Fix Committed',
	])
	for task in tasks:
		if args.n is not None and num_bugs >= args.n:
			break

		if args.show_progress:
			print('.', file=sys.stderr, end='')

		bug = task.bug
		num_bugs += 1

		last_updated = bug.date_last_updated or bug.date_created
		last_updated_map[bug.id] = last_updated

		def handle_ancient():
			# Expired check shouldn't be needed as searchTasks should filter
			# those out, but play it safe.
			if task.status == 'Expired':
				return False
			if last_updated < now - datetime.timedelta(days=365*5):
				if task.milestone_link:
					print(f'Ancient bug has milestone: {bug.web_link} {bug.title[:60]!r}: {task.milestone_link}')
				ancients.append(bug)
				if args.update:
					print(f'Updating ancient bug to Expired: {bug.web_link} {bug.title[:60]!r}')
					task.status = 'Expired'
					task.lp_save()
					if BOT_TAG not in bug.tags:
						bug.tags += [BOT_TAG]
						bug.lp_save()
					content = "This bug has not been updated in 5 years, so we're marking it Expired. If you believe this is incorrect, please update the status."
					bug.newMessage(content=content, send_notifications=send_notifications)
				else:
					print(f'WOULD Update ancient bug to Expired: {bug.web_link} {bug.title[:60]!r}')
				return True
			return False

		# Check for ancient bugs using the simple attributes (faster)
		if handle_ancient():
			continue

		if BOT_TAG in bug.tags:
			# Re-compute last-updated time but filter out this script's changes
			orig_last_updated = last_updated
			last_updated = recompute_last_updated(bug, self_link)
			last_updated_map[bug.id] = last_updated

			if last_updated != orig_last_updated:
				if args.verbose:
					print(f'Recomputed last_updated ignoring bot changes: {bug.web_link}')
				# Need to do this again after we've recalculated last_updated
				if handle_ancient():
					continue

		if task.importance not in ['Low', 'Wishlist'] and last_updated < now - datetime.timedelta(days=365*2):
			if task.milestone_link:
				print(f'Old bug has milestone: {bug.web_link} {bug.title[:60]!r}: {task.milestone_link}')
			olds.append(bug)
			if args.update:
				print(f'Updating old bug to Low: {bug.web_link} {bug.title[:60]!r}')
				task.importance = 'Low'
				task.lp_save()
				if BOT_TAG not in bug.tags:
					bug.tags += [BOT_TAG]
					bug.lp_save()
				content = "This bug has not been updated in 2 years, so we're marking it Low importance. If you believe this is incorrect, please update the importance."
				bug.newMessage(content=content, send_notifications=send_notifications)
			else:
				print(f'WOULD Update old bug to Low: {bug.web_link} {bug.title[:60]!r}')
			continue

		if task.importance == 'Medium' and last_updated < now - datetime.timedelta(days=60):
			if task.milestone_link:
				print(f'Aging bug has milestone: {bug.web_link} {bug.title[:60]!r}: {task.milestone_link}')
			aging.append(bug)
			if args.update:
				print(f'Updating aging bug to Low: {bug.web_link} {bug.title[:60]!r}')
				task.importance = 'Low'
				task.lp_save()
				if BOT_TAG not in bug.tags:
					bug.tags += [BOT_TAG]
					bug.lp_save()
				content = "This Medium-priority bug has not been updated in 60 days, so we're marking it Low importance. If you believe this is incorrect, please update the importance."
				bug.newMessage(content=content, send_notifications=send_notifications)
			else:
				print(f'WOULD Update aging bug to Low: {bug.web_link} {bug.title[:60]!r}')
			continue

		if task.importance in ('High', 'Critical'):
			high.append(bug)
			continue

		other.append(bug)

	print(file=sys.stderr)
	total_time = time.time() - start

	for category, bugs in [
		('Ancient', ancients),
		('Old', olds),
		('Aging', aging),
		('High', high),
		('Other', other),
	]:
		print(f'{category}: {len(bugs)}:')
		if args.verbose:
			for bug in bugs:
				last_updated = last_updated_map.get(bug.id)
				print(f'    {bug.id:7} {last_updated} {bug.title[:60]!r}')

	print('Total time:', total_time, file=sys.stderr)


def recompute_last_updated(bug, self_link):
	message_dates = [m.date_last_edited or m.date_created
	                 for m in bug.messages
	                 if m.date_deleted is None and m.owner_link != self_link]
	activity_dates = [a.datechanged for a in bug.activity
	                  if a.person_link != self_link]
	all_dates = sorted(message_dates + activity_dates)
	if all_dates:
		last_updated = all_dates[-1]
	else:
		last_updated = bug.date_last_updated or bug.date_created
	return last_updated


if __name__ == '__main__':
	main()
