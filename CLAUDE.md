# krocli

Kroger API CLI tool built with Go + Kong.

## Build & Run
```bash
make build        # → bin/krocli
make lint         # golangci-lint
```

## Architecture
- `cmd/krocli/main.go` → `internal/cmd.Execute()`
- Kong CLI with subcommands: auth, products, locations, cart, identity
- Two auth modes: client_credentials (search) and authorization_code (cart/identity)
- Tokens stored in OS keyring via 99designs/keyring
- Credentials in `~/.config/krocli/credentials.json`
- Output: `-j` JSON, `-p` plain/TSV, default human-friendly to stderr
