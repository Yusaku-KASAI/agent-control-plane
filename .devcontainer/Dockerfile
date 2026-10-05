# syntax=docker/dockerfile:1

# COPY --from cannot expand build args, so pinned upstream images are aliased as stages here.
ARG UV_VERSION=0.12.20
ARG GO_VERSION=1.27.1
FROM ghcr.io/astral-sh/uv:${UV_VERSION} AS uv-source
FROM golang:${GO_VERSION} AS go-source

FROM node:24 AS base

ARG TZ
ENV TZ="$TZ"
ENV DEVCONTAINER=true
ARG CLAUDE_CODE_VERSION=latest

# Package lists are kept so a human can `docker exec -u root` and apt-get install ad hoc without a
# rebuild. `node` gets no sudo for apt.
RUN apt-get update && apt-get install -y --no-install-recommends \
    less \
    git \
    procps \
    sudo \
    fzf \
    zsh \
    man-db \
    unzip \
    gnupg2 \
    gh \
    iptables \
    ipset \
    iproute2 \
    dnsutils \
    aggregate \
    jq \
    nano \
    vim \
    tmux \
    lsof \
    tcpdump \
    python3 \
    python3-venv \
    shellcheck \
    shfmt \
    rsyslog \
    locales \
    && apt-get clean

RUN echo 'en_US.UTF-8 UTF-8' >> /etc/locale.gen \
    && echo 'ja_JP.UTF-8 UTF-8' >> /etc/locale.gen \
    && locale-gen
ENV LANG=en_US.UTF-8 LANGUAGE=en_US:en LESSCHARSET=utf-8

ARG REPO_NAME
# Read by docker-entrypoint.sh for UID/GID alignment.
ENV WORKSPACE_DIR=/workspace/${REPO_NAME}

RUN mkdir -p /usr/local/share/npm-global && chown -R node:node /usr/local/share
WORKDIR /workspace/${REPO_NAME}

# Wired via the SYSTEM gitconfig so it survives the ~/.gitconfig VS Code copies in from the host.
ARG GIT_DELTA_VERSION=0.18.2
RUN ARCH=$(dpkg --print-architecture) && \
    wget "https://github.com/dandavison/delta/releases/download/${GIT_DELTA_VERSION}/git-delta_${GIT_DELTA_VERSION}_${ARCH}.deb" && \
    dpkg -i "git-delta_${GIT_DELTA_VERSION}_${ARCH}.deb" && \
    rm "git-delta_${GIT_DELTA_VERSION}_${ARCH}.deb" && \
    git config --system core.pager delta && \
    git config --system interactive.diffFilter "delta --color-only" && \
    git config --system delta.navigate true && \
    git config --system delta.line-numbers true

COPY --from=uv-source /uv /uvx /usr/local/bin/

USER node

ENV NPM_CONFIG_PREFIX=/usr/local/share/npm-global
ENV PATH=$PATH:/usr/local/share/npm-global/bin
RUN npm install -g @anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}

ENV SHELL=/bin/zsh
ARG ZSH_IN_DOCKER_VERSION=1.2.0
RUN sh -c "$(wget -O- https://github.com/deluan/zsh-in-docker/releases/download/v${ZSH_IN_DOCKER_VERSION}/zsh-in-docker.sh)" -- \
    -p git \
    -p fzf \
    -a "source /usr/share/doc/fzf/examples/key-bindings.zsh" \
    -a "source /usr/share/doc/fzf/examples/completion.zsh" \
    -a "export PROMPT_COMMAND='history -a' && export HISTFILE=/commandhistory/.bash_history" \
    -x
COPY --chown=node:node ./.devcontainer/tmux.conf /home/node/.tmux.conf

ENV EDITOR=nano
ENV VISUAL=nano

USER root
# Scripts stay root-owned: init-firewall.sh is a `sudo NOPASSWD` target, so node must not be
# able to write it.
COPY ./.devcontainer/init-firewall.sh \
     ./.devcontainer/fix-permissions.sh \
     ./.devcontainer/background-sync.sh \
     ./.devcontainer/sni-capture.py \
     ./.devcontainer/fw-show-blocked.sh \
     ./.devcontainer/docker-entrypoint.sh \
     /usr/local/bin/
RUN chmod +x /usr/local/bin/init-firewall.sh /usr/local/bin/fix-permissions.sh \
             /usr/local/bin/background-sync.sh /usr/local/bin/sni-capture.py \
             /usr/local/bin/fw-show-blocked.sh /usr/local/bin/docker-entrypoint.sh && \
    usermod -aG adm node && \
    printf 'node ALL=(root) NOPASSWD: /usr/local/bin/init-firewall.sh\n' \
        > /etc/sudoers.d/node-firewall && \
    chmod 0440 /etc/sudoers.d/node-firewall

# Final user stays root: the entrypoint needs usermod + iptables.
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["sleep", "infinity"]

FROM base AS go

COPY --from=go-source /usr/local/go /usr/local/go
ENV PATH=/usr/local/go/bin:/home/node/go/bin:$PATH \
    GOTOOLCHAIN=local

# Installed to /usr/local/bin, not GOPATH/bin: GOPATH is partly volume-backed, and tools there
# would drift from the versions pinned here.
ARG GOPLS_VERSION=v0.23.0
ARG DELVE_VERSION=v1.27.2
ARG GOLANGCI_LINT_VERSION=v2.14.0
ARG GOVULNCHECK_VERSION=v1.8.0
RUN set -eux; \
    export GOPATH=/tmp/gopath GOCACHE=/tmp/gocache GOBIN=/usr/local/bin; \
    go install golang.org/x/tools/gopls@${GOPLS_VERSION}; \
    go install github.com/go-delve/delve/cmd/dlv@${DELVE_VERSION}; \
    go install golang.org/x/vuln/cmd/govulncheck@${GOVULNCHECK_VERSION}; \
    curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh \
        | sh -s -- -b /usr/local/bin ${GOLANGCI_LINT_VERSION}; \
    rm -rf /tmp/gopath /tmp/gocache; \
    go version && gopls version && dlv version && golangci-lint version && govulncheck -version
