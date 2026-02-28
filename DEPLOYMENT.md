# Deployment Guide

Production crowment for crow.watch on a single Linux server (Ubuntu) with Docker Compose, Nginx, and Let's Encrypt.

## Prerequisites

- A Linux server with a public IP (e.g. Linode, Hetzner)
- A domain pointing to the server (`A` record for `crow.watch`)
- Docker and Docker Compose installed
- Nginx installed on the host (`apt install nginx`)

## 1. Server Hardening

### Create a crow user

Run these as root on a fresh server:

```bash
adduser crow
usermod -aG sudo crow
usermod -aG docker crow
```

Copy your SSH key from your local machine:

```bash
ssh-copy-id crow@<server-ip>
```

Verify you can log in as `crow` before locking down SSH.

### SSH

Edit `/etc/ssh/sshd_config`:

```
PermitRootLogin no
PasswordAuthentication no
AllowUsers crow
```

Restart: `systemctl restart sshd`

### Firewall

```bash
ufw default deny incoming
ufw default allow outgoing
ufw allow 22/tcp    # SSH
ufw allow 80/tcp    # HTTP (for ACME challenges)
ufw allow 443/tcp   # HTTPS
ufw enable
```

### Automatic Security Updates

```bash
apt install unattended-upgrades
dpkg-reconfigure -plow unattended-upgrades
```

## 2. Deploy Files

Copy `docker-compose.yml` and `.env` to the server:

```bash
mkdir -p ~/crow.watch
scp docker-compose.yml crow@<server-ip>:~/crow.watch/
```

On the server, create and edit `.env`:

```bash
cd ~/crow.watch
cp .env.example .env
chmod 600 .env
```

Edit `.env` with production values. See [.env.example](.env.example).

```bash
# Public URL — used in emails and redirects
APP_URL=https://crow.watch

# Must be true in production — cookies require HTTPS
SECURE_COOKIES=true

# Use a strong random password (e.g. openssl rand -hex 24)
POSTGRES_PASSWORD=<strong-random-password>

# Bind only to localhost — Nginx will proxy
HOST_PORT=127.0.0.1:8080
```

Important: `HOST_PORT=127.0.0.1:8080` ensures the app is only reachable through Nginx, not directly from the internet.

## 3. Nginx

### Install Certbot

```bash
apt install certbot python3-certbot-nginx
```

### Create site config

```bash
cat > /etc/nginx/sites-available/crow.watch << 'EOF'
server {
    listen 80;
    listen [::]:80;
    server_name crow.watch;

    location / {
        return 301 https://$host$request_uri;
    }
}

server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name crow.watch;

    ssl_certificate     /etc/letsencrypt/live/crow.watch/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/crow.watch/privkey.pem;

    # TLS hardening
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384;
    ssl_prefer_server_ciphers off;
    ssl_session_timeout 1d;
    ssl_session_cache shared:SSL:10m;
    ssl_session_tickets off;

    # OCSP stapling
    ssl_stapling on;
    ssl_stapling_verify on;
    resolver 1.1.1.1 8.8.8.8 valid=300s;

    # Security headers
    add_header Strict-Transport-Security "max-age=63072000; includeSubDomains; preload" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-Frame-Options "DENY" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;
    add_header Permissions-Policy "camera=(), microphone=(), geolocation=()" always;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        proxy_connect_timeout 5s;
        proxy_send_timeout 10s;
        proxy_read_timeout 30s;
    }

    # Block dotfiles
    location ~ /\. {
        deny all;
    }

    # Request limits
    client_max_body_size 1m;
}
EOF
```

### Enable and get certificate

```bash
ln -s /etc/nginx/sites-available/crow.watch /etc/nginx/sites-enabled/
rm -f /etc/nginx/sites-enabled/default

# Get certificate (temporarily comment out the ssl server block first,
# or use standalone mode)
certbot certonly --nginx -d crow.watch

# Test and reload
nginx -t
systemctl reload nginx
```

Certbot auto-renews via a systemd timer. Verify: `systemctl status certbot.timer`

## 4. Start the Application

```bash
cd ~/crow.watch
docker compose pull
docker compose up -d
```

This pulls pre-built images from GHCR and starts `db`, `migrate`, `app`, and `backup`. Check logs:

```bash
docker compose logs -f app       # app logs
docker compose logs -f backup    # backup logs (should show immediate backup on start)
docker compose logs migrate      # migration output
```

## 5. Create First User

```bash
docker compose run --rm cmd useradm add -username admin -email admin@crow.watch
```

## 6. Admin Commands

```bash
# Database shell
docker compose exec db psql -U crow -d crow_watch

# Reset a password
docker compose run --rm cmd useradm passwd -user admin

# Recalculate scores
docker compose run --rm cmd votecalc

# Seed tags/stories
docker compose run --rm cmd tagseed
docker compose run --rm cmd storyseed

# Trigger a manual backup
docker compose exec backup backup.sh

# Re-run migrations (pull latest image first)
docker compose pull migrate
docker compose up -d migrate
```

## 7. Updating

```bash
cd ~/crow.watch
docker compose pull
docker compose up -d
```

## 8. Backups

Backups run daily at 3am UTC (configurable via `BACKUP_SCHEDULE`). Each backup is a gzipped `pg_dump` uploaded to Linode
Object Storage. Backups older than 30 days are automatically deleted.

### Restore from backup

```bash
# Download the backup
linode-cli obj get crow-watch-backups backups/crow_watch_2026-02-28_030000.sql.gz

# Restore
gunzip -c crow_watch_2026-02-28_030000.sql.gz | \
  docker compose exec -T db psql -U crow -d crow_watch
```
