FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod ./
COPY main.go ./
RUN CGO_ENABLED=0 go build -o /mock-cp .

FROM alpine:3
COPY --from=build /mock-cp /mock-cp
ENTRYPOINT ["/mock-cp"]
