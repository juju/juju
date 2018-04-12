PWD := $(shell pwd)
HOOKS_DIR := $(PWD)/hooks
TEST_DIR := $(PWD)/hooks/tests
CHARM_DIR := $(PWD)


build: test proof

revision:
	@test -f revision || echo 0 > revision

proof: revision
	@echo Proofing charm...
	@(charm proof $(PWD) || [ $$? -eq 100 ]) && echo OK
	@test `cat revision` = 0 && rm revision

test:
	@echo no tests yet...

clean:
	rm -rf .venv


.PHONY: revision proof test lint sourcedeps charm-payload
