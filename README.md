# tsddns

Automatically configure Tailscale split DNS by resolving service names and device hostnames to IPs.

Supports:
- Service names (`svc:*`) via the Tailscale Services API
- Device hostnames (`device:*`) via the Devices API
- Direct IP addresses
- OAuth or API key auth
- Daemon mode for continuous updates

## Installation

```bash
go mod download
go build -o tsddns
```

## Development

### Running Tests

```bash
go test -v -cover
```

### Running Tests with Race Detection

```bash
go test -v -race -coverprofile=coverage.out
```

### View Coverage Report

```bash
go tool cover -html=coverage.out
```

## Configuration

Create a `config.json` file mapping domains to nameservers. Nameservers can be:
- Tailscale service names (e.g., `svc:my-service`)
- Tailscale device hostnames (e.g., `device:my-router`)
- Direct IP addresses (e.g., `192.168.1.1`)

Example:

```json
{
  "example.com": [
    "svc:my-gateway"
  ],
  "internal.example.com": [
    "192.168.1.1",
    "device:my-router"
  ],
  "other.com": [
    "svc:other-gateway"
  ]
}
```

## Usage

### Using API Key

```bash
export TAILSCALE_API_KEY="tskey-api-xxxxx"
./tsddns --config config.json
```

Or specify a tailnet explicitly:

```bash
export TAILSCALE_API_KEY="tskey-api-xxxxx"
./tsddns --tailnet example.com --config config.json
```

### Using OAuth Client Credentials

Create an OAuth client in your Tailscale admin console with the scopes: `devices:core:read`, `services:read`, and `dns`.

```bash
export TAILSCALE_CLIENT_ID="your-client-id"
export TAILSCALE_CLIENT_SECRET="your-client-secret"
./tsddns --config config.json
```

### Command Line Options

- `--tailnet`: Your Tailscale tailnet name (default: `-` which uses your default tailnet)
- `--config`: Path to config.json (default: `/config.json`)
- `--api-key`: Tailscale API key (or set `TAILSCALE_API_KEY` env var)
- `--client-id`: OAuth client ID (or set `TAILSCALE_CLIENT_ID` env var)
- `--client-secret`: OAuth client secret (or set `TAILSCALE_CLIENT_SECRET` env var)
- `--base-url`: Tailscale API base URL (default: `https://api.tailscale.com`)
- `--interval`: Run continuously with this interval (e.g., `5m`, `1h`, `30s`). If not set, runs once and exits.

## How It Works

Reads your config.json and resolves any `svc:` or `device:` entries to their current IPs, then updates your tailnet's split DNS config. Direct IPs are passed through unchanged.

## Required Permissions

### API Key
Your API key needs access to:
- DNS management (to update split DNS)
- Services read access (to resolve service IPs)
- Devices read access (to resolve device IPs)

### OAuth Client
Required scopes:
- `devices:core:read`
- `services:read`
- `dns`

## Example Output

```
2024/01/15 10:00:00 Using API key authentication
2024/01/15 10:00:00 Resolving service svc:my-gateway for domain example.com...
2024/01/15 10:00:00   Resolved svc:my-gateway to 100.64.0.1
2024/01/15 10:00:00 Resolving device my-router for domain internal.example.com...
2024/01/15 10:00:00   Resolved device:my-router to 100.64.0.5
2024/01/15 10:00:00 Updating split DNS configuration with 3 domains...
2024/01/15 10:00:00   example.com -> [100.64.0.1]
2024/01/15 10:00:00   internal.example.com -> [192.168.1.1 100.64.0.5]
2024/01/15 10:00:00   other.com -> [100.64.0.2]
2024/01/15 10:00:00 Successfully updated split DNS configuration
```

## Running as a Daemon

To run continuously and update DNS at regular intervals:

```bash
./tsddns --interval 5m
```

This will update split DNS immediately on start, then every 5 minutes thereafter.

## Docker

```bash
docker run -d \
  --name tsddns \
  -v /path/to/config.json:/config/config.json:ro \
  -e TAILSCALE_API_KEY="tskey-api-xxxxx" \
  ghcr.io/rajsinghtech/tsddns:latest --interval 5m
```

Or with OAuth:

```bash
docker run -d \
  --name tsddns \
  -v /path/to/config.json:/config/config.json:ro \
  -e TAILSCALE_CLIENT_ID="your-client-id" \
  -e TAILSCALE_CLIENT_SECRET="your-client-secret" \
  tsddns --interval 5m
```

## License

MIT
