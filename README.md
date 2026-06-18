# ddns

A minimalist self-hosted dynamic DNS server that maps domain names to the IP address of the last authorized HTTP requester.

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `BASIC_AUTH_USER_HASH` | — | Required. Basic auth credentials in the form `username:sha256hex(password)` |
| `DOMAINS` | — | Required. Comma-separated list of domain names this server will accept updates for |
| `HTTP_LISTEN` | `127.0.0.1:50505` | Address the HTTP update server listens on |
| `DNS_LISTEN` | `:53` | Address the DNS server listens on |
| `NO_TRUST_PROXY` | — | Set to `1` to disable trusting the `X-Forwarded-For` header (trusted by default) |
| `CACHE_FILE` | — | Path to a JSON file for persisting IP mappings across restarts |

## DNS types

This DNS server only serves `A` and `AAAA` records.

## Example

Generate the password hash:

```sh
$ ddns -genhash
Username: admin
Password: hunter2
BASIC_AUTH_USER_HASH=admin:f52fbd32b2b3b86ff88ef6c490628285f482af15ddcb29541f94bcf526a3f6c7
```

Add the output to your environment file:

```sh
# /etc/ddns/env
BASIC_AUTH_USER_HASH=admin:f52fbd32b2b3b86ff88ef6c490628285f482af15ddcb29541f94bcf526a3f6c7
DOMAINS=home.example.com
CACHE_FILE=/etc/ddns/cache.json
#NO_TRUST_PROXY=1
```

To update your IP, make an authenticated request to the HTTP server. The server determines whether to update the `A` (IPv4) or `AAAA` (IPv6) record based on the IP address of the requester — so to update both records, make two requests, one over each protocol:

```sh
# updates the A record (IPv4)
curl -4 -u admin:hunter2 "http://your-vps:50505/?domain=home.example.com"

# updates the AAAA record (IPv6)
curl -6 -u admin:hunter2 "http://your-vps:50505/?domain=home.example.com"
```

The DNS server will then answer `A` and `AAAA` queries for `home.example.com` with the respective last-seen IP addresses.

## Configuring Nginx to reverse proxy to ddns

If you have nginx with an SSL cert, you can add a `location` block to your SSL-protected `server` block that forwards HTTP traffic to `ddns`.

The `NO_TRUST_PROXY=1` setting must be disabled for this nginx config to work since it ignores `X-Forwarded-For`.

```
    location /ddns/ {
            proxy_pass http://127.0.0.1:50505;
            proxy_set_header X-Forwarded-For $remote_addr;
    }
```

## Configuring cron to update ddns every hour

Now from the network whose public IP address you want to have dynamically updated, you can configure `crond` to update your `ddns`.

_This example assumes you're using some kind of reverse-proxy config to `/ddns/` path similar to the nginx config described above._

Write this script to `/etc/cron.hourly/update-ddns` and don't forget to `chmod +x /etc/cron.hourly/update-ddns`.

```sh
#!/bin/sh
USER_PASS="admin:hunter2"
DDNS_URL="https://example.com/ddns/?domain=home.example.com"

# update ipv4 A record for home.example.com
curl -4 -s -u "${USER_PASS}" "${DDNS_URL}" >>/var/log/cron.update-ddns.log 2>&1

# update ipv6 AAAA record for home.example.com
curl -6 -s -u "${USER_PASS}" "${DDNS_URL}" >>/var/log/cron.update-ddns.log 2>&1
```

Then in `/etc/crontab` add the following to update `ddns` every hour

```
00 * * * * /etc/cron.hourly/update-ddns
```

And check `/var/log/cron.update-ddns.log`.

## Delegating a Subdomain to ddns

To make the internet resolve your ddns-managed names, delegate a subdomain to your VPS by adding two records at your registrar or existing DNS host.

First, register your VPS as a named nameserver (sometimes called a "host record" or "glue record" — the exact UI varies by registrar):

```
ns1.example.com.  A  <your VPS IP>
```

Then add an NS record delegating the subdomain:

```
home.example.com.  NS  ns1.example.com.
```

The glue record is required because `ns1.example.com` lives under `example.com` — without it, resolving the nameserver's address would create a circular dependency.

Once propagated, any DNS query for `home.example.com` will be forwarded directly to your VPS on port 53, where ddns will answer it with the last registered IP.

## How to install on systemd

The `Makefile` provides a build, clean and install suite of targets that will use the example configs to install `ddns` in a `systemd` environment.

```sh
make && make install
```

To test the install on a non-systemd environment, you can run the following:

```sh
make && make install INSTALL_PREFIX=tmp DDNS_USER=${USER} DDNS_GROUP=${USER} SYSTEMCTL=:
```
