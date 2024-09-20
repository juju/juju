"""Check the api client imports."""

import sys
import getopt

def check_all(argv):
	try:
		opts, _ = getopt.getopt(argv[1:], 'ha:d:g:', ['help', "allowed=", "disallowed=", "got="])
	except:
		print("{0} -a <allowed> -d <disallowed> -g <got>".format(argv[0]))
		sys.exit(2)

	allowed = []
	disallowed = []
	got = []
	for opt, arg in opts:
		if opt in ("-h", "--help"):
			print("{0} -a <allowed>".format(argv[0]))
			sys.exit()
		elif opt in ("-a", "--allowed"):
			allowed = arg.split("\n")
		elif opt in ("-d", "--disallowed"):
			disallowed = arg.split("\n")
		elif opt in ("-g", "--got"):
			got = arg.split("\n")

	if len(allowed) == 0 and len(disallowed) == 0:
		print("No allowed or disallowed imports specified")
		sys.exit(1)
	if len(allowed) > 0 and len(disallowed) > 0:
		print("Cannot specify both allowed and disallowed imports")
		sys.exit(1)
	if len(got) == 0:
		print("No imports found")
		sys.exit(1)

	if len(allowed) > 0:
		for g in got:
			matched = False
			for a in allowed:
				if g.startswith(a):
					matched = True
					break
			if matched == False:
				print(f"Import found: {g}")
				print("Consult the list of allowed imports.")
				sys.exit(1)
	elif len(disallowed) > 0:
		for g in got:
			for d in disallowed:
				if g.startswith(d):
					print(f"Disallowed import found: {g}")
					sys.exit(1)
	sys.exit(0)

if __name__ == '__main__':
	check_all(sys.argv)