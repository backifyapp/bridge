# Imagem oficial do Backify Bridge com suporte a Docker (modo docker).
# Inclui o docker CLI, que o helper usa para exportar volumes/inspecionar
# containers. Monte o socket do Docker e o config:
#
#   docker run -d --name backify-bridge \
#     -v /var/run/docker.sock:/var/run/docker.sock \
#     -v /etc/backify-bridge:/etc/backify-bridge \
#     backifyapp/bridge
#
# Acesso ao socket do Docker é ROOT-EQUIVALENTE no host — use só em servidores
# onde você confia no controle do Backify sobre o Docker.
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=docker" -o /backify-bridge ./cmd/backify-bridge

FROM alpine:3
RUN apk add --no-cache ca-certificates docker-cli
COPY --from=build /backify-bridge /usr/local/bin/backify-bridge
ENTRYPOINT ["backify-bridge"]
CMD ["run"]
