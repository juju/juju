# Minimal makefile for Sphinx documentation
#
# Add your customisation to `Makefile` instead.

# You can set these variables from the command line, and also
# from the environment for the first two.
SPHINXDIR       = .sphinx
SPHINXOPTS      ?= -c . -d $(SPHINXDIR)/.doctrees -j auto
SPHINXBUILD     ?= $(VENVDIR)/bin/sphinx-build
SOURCEDIR       = .
BUILDDIR        = _build
VENVDIR         = $(SPHINXDIR)/venv
PA11Y           = $(SPHINXDIR)/node_modules/pa11y/bin/pa11y.js --config $(SPHINXDIR)/pa11y.json
VENV            = $(VENVDIR)/bin/activate
TARGET          = *
ALLFILES        =  *.rst **/*.rst
METRICSDIR      = $(SOURCEDIR)/.sphinx/metrics
REQPDFPACKS     = latexmk fonts-freefont-otf texlive-latex-recommended texlive-latex-extra texlive-fonts-recommended texlive-font-utils texlive-lang-cjk texlive-xetex plantuml xindy tex-gyre dvipng
CONFIRM_SUDO    ?= N
VALE_CONFIG     = $(SPHINXDIR)/vale.ini
SPHINX_HOST     ?= 127.0.0.1
SPHINX_PORT     ?= 8000

# Put it first so that "make" without argument is like "make help".
help:
	@echo
	@echo "-------------------------------------------------------------"
	@echo "* watch, build and serve the documentation:  make run"
	@echo "* only build:                                make html"
	@echo "* only serve:                                make serve"
	@echo "* clean built doc files:                     make clean-doc"
	@echo "* clean full environment:                    make clean"
	@echo "* check links:                               make linkcheck"
	@echo "* check markdown:                            make lint-md"
	@echo "* check spelling:                            make spelling"
	@echo "* check spelling (without building again):   make spellcheck"
	@echo "* check inclusive language:                  make woke"
	@echo "* check accessibility:                       make pa11y"
	@echo "* check style guide compliance:              make vale"
	@echo "* check style guide compliance on target:    make vale TARGET=*"
	@echo "* check metrics for documentation:           make allmetrics"
	@echo "* other possible targets:                    make <TAB twice>"
	@echo "-------------------------------------------------------------"
	@echo

.PHONY: help full‑help html epub pdf linkcheck spelling spellcheck woke \
        vale pa11y run serve install pa11y‑install   \
        vale‑install pdf‑prep pdf‑prep‑force clean clean‑doc allmetrics \
        update lint-md \
		convert-excalidraw-to-svg

full-help: $(VENVDIR)
	@. $(VENV); $(SPHINXBUILD) -M help "$(SOURCEDIR)" "$(BUILDDIR)" $(SPHINXOPTS) $(O)
	@echo "\n\033[1;31mNOTE: This help texts shows unsupported targets!\033[0m"
	@echo "Run 'make help' to see supported targets."

# If requirements are updated, venv should be rebuilt and timestamped.
$(VENVDIR):
	@echo "... setting up virtualenv"
	python3 -m venv $(VENVDIR) || { echo "You must install python3-venv before you can build the documentation."; exit 1; }
	. $(VENV); pip install $(PIPOPTS) --require-virtualenv \
	    --upgrade -r requirements.txt \
            --log $(VENVDIR)/pip_install.log
	@test ! -f $(VENVDIR)/pip_list.txt || \
            mv $(VENVDIR)/pip_list.txt $(VENVDIR)/pip_list.txt.bak
	@. $(VENV); pip list --local --format=freeze > $(VENVDIR)/pip_list.txt
	@touch $(VENVDIR)

pa11y-install:
	@command -v $(PA11Y) >/dev/null || { \
			echo "Installing \"pa11y\" from npm..."; echo; \
			mkdir -p $(SPHINXDIR)/node_modules/ ; \
			npm install --prefix $(SPHINXDIR) pa11y; \
		}

convert-excalidraw-to-svg:
	bash scripts/convert-excalidraw-to-svg-recursively.sh

autogenerate-docs:
	python3 scripts/autogenerate-docs.py

pymarkdownlnt-install:
	@. $(VENV); test -d $(SPHINXDIR)/venv/lib/python*/site-packages/pymarkdown || pip install pymarkdownlnt

install: $(VENVDIR)

run: install
	. $(VENV); $(VENVDIR)/bin/sphinx-autobuild -b dirhtml --host $(SPHINX_HOST) --port $(SPHINX_PORT) \
	 --pre-build "bash scripts/convert-excalidraw-to-svg-recursively.sh" \
	 --pre-build "$(MAKE) -C ../ go-install" --pre-build "python3 scripts/autogenerate-docs.py" \
	 "$(SOURCEDIR)" "$(BUILDDIR)" $(SPHINXOPTS)

# Does not depend on $(BUILDDIR) to rebuild properly at every run.
html: install convert-excalidraw-to-svg
	. $(VENV); $(SPHINXBUILD) --keep-going -b dirhtml "$(SOURCEDIR)" "$(BUILDDIR)" -w $(SPHINXDIR)/warnings.txt $(SPHINXOPTS)

epub: install
	. $(VENV); $(SPHINXBUILD) -b epub "$(SOURCEDIR)" "$(BUILDDIR)" -w $(SPHINXDIR)/warnings.txt $(SPHINXOPTS)

serve: html
	cd "$(BUILDDIR)"; python3 -m http.server --bind 127.0.0.1 8000

clean: clean-doc
	@test ! -e "$(VENVDIR)" -o -d "$(VENVDIR)" -a "$(abspath $(VENVDIR))" != "$(VENVDIR)"
	rm -rf $(VENVDIR)
	rm -rf $(SPHINXDIR)/node_modules/
	rm -rf $(SPHINXDIR)/styles
	rm -rf $(VALE_CONFIG)

clean-doc:
	git clean -fx "$(BUILDDIR)"
	rm -rf $(SPHINXDIR)/.doctrees

linkcheck: install
	. $(VENV) ; $(SPHINXBUILD) -b linkcheck "$(SOURCEDIR)" "$(BUILDDIR)" $(SPHINXOPTS) || { grep --color -F "[broken]" "$(BUILDDIR)/output.txt"; exit 1; }
	exit 0

pa11y: pa11y-install html
	find $(BUILDDIR) -name *.html -print0 | xargs -n 1 -0 $(PA11Y)

lint-md: pymarkdownlnt-install
	@. $(VENV); pymarkdownlnt --config $(SPHINXDIR)/.pymarkdown.json scan --recurse --exclude=./$(SPHINXDIR)/** $(SOURCEDIR)

vale-install: install
	@. $(VENV); test -d $(SPHINXDIR)/venv/lib/python*/site-packages/vale || pip install rst2html vale
	@. $(VENV); test -f $(VALE_CONFIG) || python3 $(SPHINXDIR)/get_vale_conf.py
	@echo '.Name=="Canonical.400-Enforce-inclusive-terms"' > $(SPHINXDIR)/styles/woke.filter
	@echo '.Level=="error" and .Name!="Canonical.500-Repeated-words" and .Name!="Canonical.000-US-spellcheck"' > $(SPHINXDIR)/styles/error.filter
	@echo '.Name=="Canonical.000-US-spellcheck"' > $(SPHINXDIR)/styles/spelling.filter
	@. $(VENV); find $(SPHINXDIR)/venv/lib/python*/site-packages/vale/vale_bin -size 195c -exec vale --version \;

woke: vale-install
	@cat $(SPHINXDIR)/styles/config/vocabularies/Canonical/accept.txt > $(SPHINXDIR)/styles/config/vocabularies/Canonical/accept_backup.txt
	@cat $(SOURCEDIR)/.custom_wordlist.txt >> $(SPHINXDIR)/styles/config/vocabularies/Canonical/accept.txt
	@echo "Running Vale acceptable term check against $(TARGET). To change target set TARGET= with make command"
	@. $(VENV); vale --config="$(VALE_CONFIG)" --filter='$(SPHINXDIR)/styles/woke.filter' --glob='*.{md,rst}' $(TARGET)
	@cat $(SPHINXDIR)/styles/config/vocabularies/Canonical/accept_backup.txt > $(SPHINXDIR)/styles/config/vocabularies/Canonical/accept.txt && rm $(SPHINXDIR)/styles/config/vocabularies/Canonical/accept_backup.txt

vale: vale-install
	@cat $(SPHINXDIR)/styles/config/vocabularies/Canonical/accept.txt > $(SPHINXDIR)/styles/config/vocabularies/Canonical/accept_backup.txt
	@cat $(SOURCEDIR)/.custom_wordlist.txt >> $(SPHINXDIR)/styles/config/vocabularies/Canonical/accept.txt
	@echo "Running Vale against $(TARGET). To change target set TARGET= with make command"
	@. $(VENV); vale --config="$(VALE_CONFIG)" --filter='$(SPHINXDIR)/styles/error.filter' --glob='*.{md,rst}' $(TARGET)
	@cat $(SPHINXDIR)/styles/config/vocabularies/Canonical/accept_backup.txt > $(SPHINXDIR)/styles/config/vocabularies/Canonical/accept.txt && rm $(SPHINXDIR)/styles/config/vocabularies/Canonical/accept_backup.txt

spelling: vale-install
	@cat $(SPHINXDIR)/styles/config/vocabularies/Canonical/accept.txt > $(SPHINXDIR)/styles/config/vocabularies/Canonical/accept_backup.txt
	@cat $(SOURCEDIR)/.custom_wordlist.txt >> $(SPHINXDIR)/styles/config/vocabularies/Canonical/accept.txt
	@echo "Running Vale against $(TARGET). To change target set TARGET= with make command"
	@. $(VENV); vale --config="$(VALE_CONFIG)" --filter='$(SPHINXDIR)/styles/spelling.filter' --glob='*.{md,rst}' $(TARGET)
	@cat $(SPHINXDIR)/styles/config/vocabularies/Canonical/accept_backup.txt > $(SPHINXDIR)/styles/config/vocabularies/Canonical/accept.txt && rm $(SPHINXDIR)/styles/config/vocabularies/Canonical/accept_backup.txt

spellcheck: spelling
	@echo "Please note that the \`make spellcheck\` command is being deprecated in favor of \`make spelling\`"

pdf-prep: install
	@for packageName in $(REQPDFPACKS); do (dpkg-query -W -f='$${Status}' $$packageName 2>/dev/null | \
        grep -c "ok installed" >/dev/null && echo "Package $$packageName is installed") && continue || \
        (echo; echo "PDF generation requires the installation of the following packages: $(REQPDFPACKS)" && \
        echo "" && echo "Run 'sudo make pdf-prep-force' to install these packages" && echo "" && echo \
        "Please be aware these packages will be installed to your system") && exit 1 ; done

pdf-prep-force:
	apt-get update
	apt-get upgrade -y
	apt-get install --no-install-recommends -y $(REQPDFPACKS) \

pdf: pdf-prep
	@. $(VENV); sphinx-build -M latexpdf "$(SOURCEDIR)" "$(BUILDDIR)" $(SPHINXOPTS)
	@rm ./$(BUILDDIR)/latex/front-page-light.pdf || true
	@rm ./$(BUILDDIR)/latex/normal-page-footer.pdf || true
	@find ./$(BUILDDIR)/latex -name "*.pdf" -exec mv -t ./$(BUILDDIR) {} +
	@rm -r $(BUILDDIR)/latex
	@echo
	@echo "Output can be found in ./$(BUILDDIR)"
	@echo

allmetrics: html
	@echo "Recording documentation metrics..."
	@echo "Checking for existence of vale..."
	. $(VENV)
	@. $(VENV); test -d $(SPHINXDIR)/venv/lib/python*/site-packages/vale || pip install vale
	@. $(VENV); test -f $(VALE_CONFIG) || python3 $(SPHINXDIR)/get_vale_conf.py
	@. $(VENV); find $(SPHINXDIR)/venv/lib/python*/site-packages/vale/vale_bin -size 195c -exec vale --config "$(VALE_CONFIG)" $(TARGET) > /dev/null \;
	@eval '$(METRICSDIR)/source_metrics.sh $(PWD)'
	@$(METRICSDIR)/build_metrics.py $(BUILDDIR)

update: install
	@. $(VENV); .sphinx/update_sp.py

# Catch-all target: route all unknown targets to Sphinx using the new
# "make mode" option.  $(O) is meant as a shortcut for $(SPHINXOPTS).
%:
	$(MAKE) —no-print-directory install
	. $(VENV); $(SPHINXBUILD) -M $@ "$(SOURCEDIR)" "$(BUILDDIR)" $(SPHINXOPTS) $(O)
