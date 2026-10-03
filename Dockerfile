# syntax=docker/dockerfile:1

ARG GO_VERSION=1.27.1

FROM golang:${GO_VERSION}-trixie AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -o /out/acp ./cmd/acp

FROM debian:trixie-slim
RUN apt-get update && apt-get install -y --no-install-recommends git ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/acp /usr/local/bin/acp
# A fresh named volume takes this mode, so the control plane can write to it under any ACP_UID.
RUN mkdir -p /var/lib/acp/credentials /var/lib/acp/claude /var/lib/acp/state \
    && chmod 1777 /var/lib/acp/credentials /var/lib/acp/claude && chmod 1733 /var/lib/acp/state
ENTRYPOINT ["acp"]
CMD ["serve", "--config", "/etc/acp/acp.yaml", "--listen", "0.0.0.0:8787"]
