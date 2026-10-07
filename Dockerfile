# exitlag-node container image. Runs with host networking and NET_ADMIN;
# see deploy/docker-compose.yml and https://altairca.github.io/ExitLagFree/node/docker

FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X github.com/AltairCA/ExitLagFree/internal/version.Version=${VERSION}" \
    -o /out/exitlag-node ./cmd/exitlag-node

FROM alpine:3
RUN apk add --no-cache nftables iptables ca-certificates tini
COPY --from=build /out/exitlag-node /usr/local/bin/exitlag-node
COPY scripts/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
VOLUME ["/etc/exitlag-node", "/var/lib/exitlag-node"]
ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/docker-entrypoint.sh"]
CMD ["serve"]
