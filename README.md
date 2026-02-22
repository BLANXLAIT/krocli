# krocli

CLI tool for the [Kroger API](https://developer.kroger.com/).

## Install

```bash
go install github.com/blanxlait/krocli/cmd/krocli@latest
```

Or build from source:

```bash
make build    # → bin/krocli
```

## Setup

1. Register an app at [developer.kroger.com](https://developer.kroger.com/) to get a client ID and secret.

2. Create a JSON file with your credentials:
   ```json
   {"client_id": "YOUR_ID", "client_secret": "YOUR_SECRET"}
   ```

3. Import them:
   ```bash
   krocli auth credentials set /path/to/creds.json
   ```

Credentials are stored at `~/.config/krocli/credentials.json`. Tokens are stored in your OS keyring.

## Authentication

There are two auth modes:

- **Client credentials** — automatic, used for product/location searches
- **Authorization code** — required for cart and identity; run `krocli auth login` to complete the browser OAuth flow

```bash
krocli auth login       # Browser OAuth → stores refresh token
krocli auth status      # Show current auth state
```

## Usage

### Products

```bash
krocli products search --term "milk"
krocli products search --term "bread" --location-id 01400376 --limit 5
krocli products get 0011110838049
```

### Locations

```bash
krocli locations search --zip-code 45202
krocli locations search --zip-code 45202 --radius 25
krocli locations get 01400376
krocli locations chains
krocli locations departments
```

### Cart (requires `auth login`)

```bash
krocli cart add --upc 0011110838049 --qty 2
```

### Identity (requires `auth login`)

```bash
krocli identity profile
```

## Output Formats

| Flag | Format | Destination |
|------|--------|-------------|
| (none) | Human-friendly | stderr |
| `-j` | JSON | stdout |
| `-p` | Plain/TSV | stdout |

Pipe-friendly: `krocli -j products search --term "eggs" | jq '.data[].description'`

## Development

```bash
make build    # Build binary
make lint     # Run golangci-lint
make clean    # Remove build artifacts
```
