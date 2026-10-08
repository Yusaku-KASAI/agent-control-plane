# syntax=docker/dockerfile:1

# COPY --from cannot expand build args, so pinned upstream images are aliased as stages here.
ARG UV_VERSION=0.12.20
ARG GO_VERSION=1.27.1
FROM ghcr.io/astral-sh/uv:${UV_VERSION} AS uv-source
FROM golang:${GO_VERSION} AS go-source

# Nothing here depends on the repository, so the stage can be shared by every target.
FROM node:24 AS base

ARG TZ=UTC
ENV TZ="$TZ"

# Package lists are kept so a human can `docker exec -u root` and apt-get install ad hoc without a
# rebuild. `node` gets no sudo.
RUN apt-get update && apt-get install -y --no-install-recommends \
    less \
    git \
    procps \
    fzf \
    zsh \
    man-db \
    unzip \
    gnupg2 \
    gh \
    jq \
    nano \
    vim \
    tmux \
    lsof \
    python3 \
    python3-venv \
    shellcheck \
    shfmt \
    locales \
    && apt-get clean

RUN echo 'en_US.UTF-8 UTF-8' >> /etc/locale.gen \
    && echo 'ja_JP.UTF-8 UTF-8' >> /etc/locale.gen \
    && locale-gen
ENV LANG=en_US.UTF-8 LANGUAGE=en_US:en LESSCHARSET=utf-8

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
COPY --chmod=755 ./.devcontainer/fix-permissions.sh ./.devcontainer/docker-entrypoint.sh /usr/local/bin/
RUN mkdir -p /usr/local/share/npm-global && chown -R node:node /usr/local/share

USER node
ENV SHELL=/bin/zsh EDITOR=nano VISUAL=nano
ARG ZSH_IN_DOCKER_VERSION=1.2.0
RUN sh -c "$(wget -O- https://github.com/deluan/zsh-in-docker/releases/download/v${ZSH_IN_DOCKER_VERSION}/zsh-in-docker.sh)" -- \
    -p git \
    -p fzf \
    -a "source /usr/share/doc/fzf/examples/key-bindings.zsh" \
    -a "source /usr/share/doc/fzf/examples/completion.zsh" \
    -x
COPY --chown=node:node ./.devcontainer/tmux.conf /home/node/.tmux.conf

ENV NPM_CONFIG_PREFIX=/usr/local/share/npm-global
ENV PATH=/home/node/.local/bin:$PATH:/usr/local/share/npm-global/bin

# Last in the stage: it changes most often.
ARG CLAUDE_CODE_VERSION=2.1.292
RUN curl -fsSL https://claude.ai/install.sh | bash -s "${CLAUDE_CODE_VERSION}" \
    && claude --version | grep -q "^${CLAUDE_CODE_VERSION} "
# Claude Code updates itself unless told not to, which would replace the pinned version. Set after
# the install, which it would also block.
ENV DISABLE_UPDATES=1

# Final user stays root: the entrypoint needs usermod and chown.
USER root
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["sleep", "infinity"]

# Go tools build apart from base, so that changing base (such as bumping Claude Code) does not
# rebuild them.
FROM go-source AS go-tools
ARG GOPLS_VERSION=v0.23.0
ARG DELVE_VERSION=v1.27.2
ARG GOLANGCI_LINT_VERSION=v2.14.0
ARG GOVULNCHECK_VERSION=v1.8.0
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg/mod \
    set -eux; \
    export GOBIN=/out CGO_ENABLED=0; \
    go install golang.org/x/tools/gopls@${GOPLS_VERSION}; \
    go install github.com/go-delve/delve/cmd/dlv@${DELVE_VERSION}; \
    go install golang.org/x/vuln/cmd/govulncheck@${GOVULNCHECK_VERSION}; \
    curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh \
        | sh -s -- -b /out ${GOLANGCI_LINT_VERSION}

FROM base AS go

COPY --from=go-source /usr/local/go /usr/local/go
ENV PATH=/usr/local/go/bin:/home/node/go/bin:$PATH \
    GOTOOLCHAIN=local
# In /usr/local/bin, not GOPATH/bin: GOPATH is partly volume-backed, and tools there would drift
# from the versions pinned here.
COPY --from=go-tools /out/ /usr/local/bin/
RUN go version && gopls version && dlv version && golangci-lint version && govulncheck -version

# Built for the host user, so that the entrypoint has nothing to realign at start. Kept out of the
# shared stages, which are built once for every host.
ARG HOST_UID=1000
ARG HOST_GID=1000
RUN if [ "$HOST_UID" != "$(id -u node)" ] || [ "$HOST_GID" != "$(id -g node)" ]; then \
        groupmod -o -g "$HOST_GID" node \
        && usermod -o -u "$HOST_UID" -g "$HOST_GID" node \
        && chown -R node:node /home/node /usr/local/share/npm-global; \
    fi

# CI runs make check as the check stage, without exporting the image. Each check copies only the
# files it reads, so its layer stays cached until one of them changes, and the three build in
# parallel.
FROM go AS check-base
USER node
WORKDIR /workspace/agent-control-plane
COPY --chown=node:node Makefile ./

FROM check-base AS check-go
COPY --chown=node:node go.mod go.sum ./
RUN go mod download
# Tests also read the example config and this repository's plugin declarations.
COPY --chown=node:node .golangci.yml acp.example.yaml ./
COPY --chown=node:node .claude .claude
COPY --chown=node:node platform platform
COPY --chown=node:node cmd cmd
COPY --chown=node:node internal internal
RUN make check-go

FROM check-base AS check-platform
COPY --chown=node:node platform platform
RUN make check-platform

FROM check-base AS check-target
COPY --chown=node:node .devcontainer .devcontainer
COPY --chown=node:node .vscode .vscode
RUN make check-target

FROM scratch AS check
COPY --from=check-go /workspace/agent-control-plane/Makefile /check-go
COPY --from=check-platform /workspace/agent-control-plane/Makefile /check-platform
COPY --from=check-target /workspace/agent-control-plane/Makefile /check-target
