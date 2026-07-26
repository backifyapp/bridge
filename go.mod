module github.com/backifyapp/bridge

go 1.25.0

// Túnel reverso (TCP sobre HTTPS, cripto SSH). Rode `go mod tidy` para popular
// o go.sum e as dependências transitivas.
require github.com/jpillora/chisel v1.11.8

require (
	github.com/armon/go-socks5 v0.0.0-20160902184237-e75332964ef5 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/jpillora/backoff v1.0.0 // indirect
	github.com/jpillora/sizestr v1.0.0 // indirect
	golang.org/x/crypto v0.52.0 // indirect
	golang.org/x/net v0.54.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
)
