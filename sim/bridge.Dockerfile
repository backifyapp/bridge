# Bridge build for the simulation. Context = root of the bridge repo.
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=sim" -o /backify-bridge ./cmd/backify-bridge

FROM alpine:3
RUN apk add --no-cache ca-certificates
COPY --from=build /backify-bridge /usr/local/bin/backify-bridge
COPY sim/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
