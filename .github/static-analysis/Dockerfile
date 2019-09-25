FROM ubuntu:18.04

LABEL "name"="Static Analysis"
LABEL "maintainer"="Juju <juju@canonical.com>"
LABEL "version"="1.0.0"

LABEL "com.github.actions.name"="Static Analysis"
LABEL "com.github.actions.description"="Action for static analysis"
LABEL "com.github.actions.icon"="filter"
LABEL "com.github.actions.color"="gray-dark"

RUN apt-get update && \
    apt-get install --no-install-recommends -y \
        build-essential \
        ca-certificates \
        curl \
        gnupg2 \
        jq \
        make \
        golang \
        shellcheck && \
    apt-get clean -y && \
    rm -rf /var/lib/apt/lists/*

ENV GOPATH="$HOME/go"
ENV PATH="$HOME/go/bin:${PATH}"

ENTRYPOINT ["/bin/make"]
CMD ["static-analysis"]
