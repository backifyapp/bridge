# Official Backify Bridge image, with Docker support (docker mode).
# Bundles the docker CLI, which the helper uses to export volumes and inspect
# containers. Mount the Docker socket and the config:
#
#   docker run -d --name backify-bridge \
#     -v /var/run/docker.sock:/var/run/docker.sock \
#     -v /etc/backify-bridge:/etc/backify-bridge \
#     ghcr.io/backifyapp/bridge
#
# Access to the Docker socket is ROOT-EQUIVALENT on the host — only use it on
# servers where you trust Backify's control over Docker.
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# The release workflow passes the tag; a local build falls back to "dev". It used
# to be hardcoded to "docker", so the dashboard showed every containerised agent
# as running version "docker" — useless for knowing what is deployed.
ARG VERSION=dev
RUN CHISEL=$(go list -m -f '{{.Version}}' github.com/jpillora/chisel | sed 's/^v//') && \
    CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSION} -X github.com/jpillora/chisel/share.BuildVersion=${CHISEL}" \
      -o /backify-bridge ./cmd/backify-bridge

FROM alpine:3
RUN apk add --no-cache ca-certificates docker-cli
COPY --from=build /backify-bridge /usr/local/bin/backify-bridge
ENTRYPOINT ["backify-bridge"]
CMD ["run"]
