"""Check the api client imports."""

import sys
import getopt

def check_all(argv):
	try:
		opts, _ = getopt.getopt(argv[1:], 'ha:g:', ['help', "allowed=", "got="])
	except:
		print("{0} -a <allowed> -g <got>".format(argv[0]))
		sys.exit(2)

	allowed = []
	got = []
	for opt, arg in opts:
		if opt in ("-h", "--help"):
			print("{0} -a <allowed>".format(argv[0]))
			sys.exit()
		elif opt in ("-a", "--allowed"):
			allowed = arg.split("\n")
		elif opt in ("-g", "--got"):
			got = arg.split("\n")
			
	if len(allowed) == 0:
		print("No allowed imports specified")
		sys.exit(1)
	if len(got) == 0:
		print("No imports found")
		sys.exit(1)

	for g in got:
		matched = False
		for a in allowed:
			if g.startswith(a):
				matched = True
				break
		if matched == False:
			print(f"Import not allowed: {g}")
			print("Consult the list of allowed imports.")
			sys.exit(1)
	sys.exit(0)

if __name__ == '__main__':
	check_all(sys.argv)