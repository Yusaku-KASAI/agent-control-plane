# syntax=docker/dockerfile:1

# COPY --from cannot expand build args, so pinned upstream images are aliased as stages here.
ARG UV_VERSION=0.12.20
ARG GO_VERSION=1.27.1
FROM ghcr.io/astral-sh/uv:${UV_VERSION} AS uv-source
FROM golang:${GO_VERSION}-trixie AS go-source

# Tools every target's agents use. Nothing here depends on the repository or the host, so the stage
# can be shared by every target. Layers are ordered from least to most often changed.
FROM node:24-trixie AS base
SHELL ["/bin/bash", "-o", "pipefail", "-c"]

# Package lists are kept so a human can `docker exec -u root` and apt-get install ad hoc without a
# rebuild.
RUN apt-get update && apt-get install -y --no-install-recommends \
        fzf gh git gnupg2 jq less locales lsof man-db nano procps python3 python3-venv shellcheck \
        shfmt tmux unzip vim zsh \
    && apt-get clean \
    && printf '%s\n' 'en_US.UTF-8 UTF-8' 'ja_JP.UTF-8 UTF-8' >>/etc/locale.gen \
    && locale-gen
ENV LANG=en_US.UTF-8 LANGUAGE=en_US:en LESSCHARSET=utf-8

# Wired via the SYSTEM gitconfig so it survives the ~/.gitconfig VS Code copies in from the host.
ARG GIT_DELTA_VERSION=0.18.2
RUN deb="git-delta_${GIT_DELTA_VERSION}_$(dpkg --print-architecture).deb" \
    && curl -fsSLo "/tmp/$deb" "https://github.com/dandavison/delta/releases/download/${GIT_DELTA_VERSION}/$deb" \
    && dpkg -i "/tmp/$deb" && rm "/tmp/$deb" \
    && git config --system core.pager delta \
    && git config --system interactive.diffFilter "delta --color-only" \
    && git config --system delta.navigate true \
    && git config --system delta.line-numbers true

COPY --from=uv-source /uv /uvx /usr/local/bin/

RUN mkdir -p /usr/local/share/npm-global && chown -R node:node /usr/local/share \
    && install -d -o node -g node /commandhistory

USER node
ENV SHELL=/bin/zsh EDITOR=nano VISUAL=nano
ARG ZSH_IN_DOCKER_VERSION=1.2.0
RUN curl -fsSLo /tmp/zsh-in-docker.sh \
        "https://github.com/deluan/zsh-in-docker/releases/download/v${ZSH_IN_DOCKER_VERSION}/zsh-in-docker.sh" \
    && sh /tmp/zsh-in-docker.sh \
        -p git \
        -p fzf \
        -a "source /usr/share/doc/fzf/examples/key-bindings.zsh" \
        -a "source /usr/share/doc/fzf/examples/completion.zsh" \
        -x \
    && rm /tmp/zsh-in-docker.sh
# Mount points and their parents are made here, owned by node: Docker would create missing ones as
# root, and nothing could then write beside the mounts (such as elsewhere in ~/.config). A new named
# volume also takes the owner of its mount point.
RUN mkdir -p /home/node/.claude /home/node/.config/gh

ENV NPM_CONFIG_PREFIX=/usr/local/share/npm-global
# Without it, Claude Code keeps part of its state in ~/.claude.json, outside the mounted directory.
ENV CLAUDE_CONFIG_DIR=/home/node/.claude
ENV PATH=/home/node/.local/bin:$PATH:/usr/local/share/npm-global/bin
ARG CLAUDE_CODE_VERSION=2.1.292
RUN curl -fsSL https://claude.ai/install.sh | bash -s "${CLAUDE_CODE_VERSION}" \
    && claude --version | grep -q "^${CLAUDE_CODE_VERSION} "
# Claude Code updates itself unless told not to, which would replace the pinned version. Set after
# the install, which it would also block.
ENV DISABLE_UPDATES=1

# Small files that change more often than the layers above.
COPY --chown=node:node ./.devcontainer/tmux.conf /home/node/.tmux.conf
COPY --chmod=755 ./.devcontainer/docker-entrypoint.sh /usr/local/bin/
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["sleep", "infinity"]

# Go tools build apart from base, so that changing base (such as bumping Claude Code) does not
# rebuild them.
FROM go-source AS go-tools
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
ARG GOPLS_VERSION=v0.23.0
ARG DELVE_VERSION=v1.27.2
ARG GOLANGCI_LINT_VERSION=v2.14.0
ARG GOVULNCHECK_VERSION=v1.8.0
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg/mod \
    export GOBIN=/out CGO_ENABLED=0 \
    && go install golang.org/x/tools/gopls@${GOPLS_VERSION} \
    && go install github.com/go-delve/delve/cmd/dlv@${DELVE_VERSION} \
    && go install golang.org/x/vuln/cmd/govulncheck@${GOVULNCHECK_VERSION} \
    && curl -fsSL "https://raw.githubusercontent.com/golangci/golangci-lint/${GOLANGCI_LINT_VERSION}/install.sh" \
        | sh -s -- -b /out "${GOLANGCI_LINT_VERSION}"

# This repository's toolchain, then the host-specific settings.
FROM base AS go
COPY --from=go-source /usr/local/go /usr/local/go
ENV PATH=/usr/local/go/bin:/home/node/go/bin:$PATH \
    GOTOOLCHAIN=local
# In /usr/local/bin, not GOPATH/bin: GOPATH is partly volume-backed, and tools there would drift
# from the versions pinned here.
COPY --from=go-tools /out/ /usr/local/bin/
RUN go version && gopls version && dlv version && golangci-lint version && govulncheck -version \
    && mkdir -p /home/node/go/pkg/mod /home/node/.cache/go-build

ARG TZ=UTC
ENV TZ="$TZ"
# The container runs as node, never as root, so node must own what is mounted for it: build with the
# workspace owner's UID/GID. The user is numeric so that platforms can check it is not root (such as
# Kubernetes runAsNonRoot). /home/node holds only what the image put there, so the chown is quick.
ARG HOST_UID=1000
ARG HOST_GID=1000
USER root
RUN if [ "$HOST_UID" != "$(id -u node)" ] || [ "$HOST_GID" != "$(id -g node)" ]; then \
        groupmod -o -g "$HOST_GID" node \
        && usermod -o -u "$HOST_UID" -g "$HOST_GID" node \
        && chown -R node:node /home/node /usr/local/share/npm-global /commandhistory; \
    fi
USER ${HOST_UID}:${HOST_GID}

# CI runs each check-<concern> stage as its own job, without exporting the image. Each copies only
# the files its check reads, so its layer stays cached until one of them changes.
FROM go AS check-base
USER node
WORKDIR /src
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
