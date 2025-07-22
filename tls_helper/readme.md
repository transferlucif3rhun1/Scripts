# TLS Client Proxy Server

## Endpoints

| Method | Endpoint   | Description                                                     |
| ------ | ---------- | --------------------------------------------------------------- |
| `ALL`  | `/`        | Main proxy endpoint - returns processed response                |
| `ALL`  | `/raw`     | Raw response - returns unprocessed server response              |
| `ALL`  | `/payload` | Payload only - returns formatted request without making request |

## Headers

### Required

| Header  | Description | Example                   |
| ------- | ----------- | ------------------------- |
| `x-url` | Target URL  | `https://example.com/api` |

### Optional

| Header               | Description      | Values                                      |
| -------------------- | ---------------- | ------------------------------------------- |
| `x-client`           | Browser profile  | `chrome`, `firefox`, `safari`, `chrome_133` |
| `x-sid`              | Session ID       | `session-123`                               |
| `x-proxy`            | Proxy config     | `ip:port:user:pass`                         |
| `x-version`          | HTTP version     | `1` (force HTTP/1.1)                        |
| `x-follow-redirects` | Handle redirects | `true`, `false`                             |
| `x-timeout`          | Timeout (ms)     | `30000`                                     |
| `x-insecure`         | Skip SSL verify  | `true`, `false`                             |
