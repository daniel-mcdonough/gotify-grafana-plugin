# Gotify Webhook Forwarder Plugin

A Gotify plugin that creates webhook endpoints to receive messages from external services and forward them to Gotify users.

## Features

- Provides a webhook endpoint for each user that can receive JSON messages
- **Automatic Grafana webhook detection and parsing**
- Forwards received messages to the user's Gotify client
- Supports message title, content, priority, and custom extras
- Built-in validation and error handling
- Info endpoint for discovering the webhook URL
- Smart priority assignment for Grafana alerts (firing=8, resolved=3)

## Usage

Once installed and enabled for a user, the plugin provides two endpoints:

### 1. Info Endpoint (GET)
```
GET /plugin/{plugin-id}/custom/{user-token}/
```
Returns information about available endpoints and how to use them.

### 2. Message Endpoint (POST)
```
POST /plugin/{plugin-id}/custom/{user-token}/message
```

The endpoint accepts two types of webhooks:

#### Generic Webhook Format
Send a JSON payload to forward a message to the Gotify user:

```json
{
  "title": "Alert Title",      // Optional, defaults to "Webhook Message"
  "message": "Message body",    // Required
  "priority": 7,                // Optional, 1-10, defaults to 5
  "extras": {                   // Optional, custom data
    "key": "value"
  }
}
```

Example using curl:
```bash
curl -X POST https://your-gotify-server/plugin/{plugin-id}/custom/{user-token}/message \
  -H "Content-Type: application/json" \
  -d '{
    "title": "System Alert",
    "message": "Disk usage exceeded 90%",
    "priority": 8
  }'
```

#### Grafana Webhook Format (Auto-detected)
The plugin automatically detects and parses Grafana webhook payloads. When Grafana sends an alert, the plugin will:

- Extract the title and message from Grafana's payload
- Set priority based on alert status:
  - `firing`/`alerting`: Priority 8 (high)
  - `resolved`/`ok`: Priority 3 (low)
  - Others: Priority 5 (default)
- Store relevant URLs (dashboard, silence, external) in extras

Grafana webhook configuration:
1. In Grafana, go to Alerting → Contact points
2. Add a new contact point with type "webhook"
3. Set the URL to: `https://your-gotify-server/plugin/{plugin-id}/custom/{user-token}/message`
4. Method: POST
5. No authentication needed (handled by Gotify user's plugin access)

## Building

Build the plugin for your Gotify server version:

```bash
# For Gotify v2.4.0
make GOTIFY_VERSION="v2.4.0" FILE_SUFFIX="for-gotify-v2.4.0" build

# For latest master
make GOTIFY_VERSION="master" build
```

This creates platform-specific `.so` files in the `build/` directory:
- `webhook-forwarder-linux-amd64.so`
- `webhook-forwarder-linux-arm-7.so`
- `webhook-forwarder-linux-arm64.so`

## Installation

1. Copy the appropriate `.so` file to your Gotify server's plugin directory
2. Restart the Gotify server
3. Enable the plugin for users who need webhook functionality
4. Find the plugin ID and user token in the Gotify web interface
5. Use the webhook URL: `https://your-server/plugin/{plugin-id}/custom/{user-token}/message`

## Use Cases

- **Grafana Alerts**: Native support for Grafana webhook notifications
- Receive alerts from monitoring systems (Prometheus, Alertmanager, etc.)
- Forward messages from CI/CD pipelines (Jenkins, GitLab CI, GitHub Actions)
- Integrate with automation tools and scripts
- Bridge messages from services that support webhooks but not Gotify
- Home automation alerts (Home Assistant, OpenHAB)
- Server monitoring and health checks

## Development

Run tests:
```bash
go test ./...
```

Update dependencies to match a specific Gotify version:
```bash
make update-go-mod GOTIFY_VERSION="v2.4.0"
```

## License

MIT