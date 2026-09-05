# CaptchaFlow Production Deployment

This guide deploys the complete platform on one Linux server with Docker
Compose, Nginx, and TLS. The API serves both the console SPA and JSON API.
PostgreSQL, Redis, and internal solver credentials stay private.

## 1. Server Prerequisites

Use a current Ubuntu LTS or Debian server with a domain name pointed at its
public IP. The commands use `captcha.example.com`; replace it with the real
domain. Install Docker Engine, the Docker Compose plugin, Nginx, Certbot, and
Git using the vendor instructions for the operating system, then verify:

```bash
docker --version
docker compose version
nginx -v
certbot --version
git --version
```

Allow only SSH, HTTP, and HTTPS at the host firewall. Do not expose ports 8000,
5432, or 6379 publicly.

```bash
sudo ufw allow OpenSSH
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw enable
sudo ufw status numbered
```

## 2. Retrieve the Release

After this repository has been pushed to GitHub, clone it to a dedicated
directory with a non-root deployment account:

```bash
sudo install -d -o "$USER" -g "$USER" /opt/captchaflow
git clone https://github.com/OWNER/captchaflow-service-platform.git /opt/captchaflow
cd /opt/captchaflow
git checkout main
git log -1 --oneline
```

For later releases, deploy a reviewed tag or commit rather than an unreviewed
branch head.

## 3. Create Runtime Configuration

The production stack reads `/opt/captchaflow/.env`. It is Git-ignored. Create
it with owner-only permissions:

```bash
cd /opt/captchaflow
cp .env.production.example .env
chmod 600 .env
```

Generate five random values on the server. Hex values are safe for both the
application secrets and `POSTGRES_PASSWORD` in the database URL:

```bash
openssl rand -hex 32
openssl rand -hex 32
openssl rand -hex 32
openssl rand -hex 32
openssl rand -hex 32
```

Edit `.env` and replace every placeholder. Keep these settings consistent:

```dotenv
POSTGRES_PASSWORD=<first-generated-value>
DATABASE_URL=postgres://captchaflow:<first-generated-value>@postgres:5432/captchaflow?sslmode=disable
SESSION_SECRET=<second-generated-value>
API_KEY_PEPPER=<third-generated-value>
CDK_PEPPER=<fourth-generated-value>
GEETEST_SOLVER_URL=<internal-solver-url-reachable-from-this-server>
GEETEST_SERVICE_API_KEY=<internal-solver-key>
TRUST_PROXY_XFF_HOPS=1
```

Set a strong one-time `ADMIN_BOOTSTRAP_PASSWORD` before the first API start.
Leave `APP_ENV=production` and `AUTO_MIGRATE=1` enabled for initial deployment.
Production mode rejects placeholder secrets and localhost service URLs. The
production Compose file exposes only `127.0.0.1:8000`; PostgreSQL and Redis
remain inside the Docker network.

## 4. Start and Verify

Build and start the services:

```bash
cd /opt/captchaflow
docker compose --env-file .env -f deploy/compose.production.yml config -q
docker compose --env-file .env -f deploy/compose.production.yml up -d --build
docker compose --env-file .env -f deploy/compose.production.yml ps
curl --fail --silent --show-error http://127.0.0.1:8000/healthz
```

If the health check fails, inspect logs before retrying:

```bash
docker compose --env-file .env -f deploy/compose.production.yml logs --tail=200 api
docker compose --env-file .env -f deploy/compose.production.yml logs --tail=100 postgres redis
```

Log in to the administrator console once. Then remove the bootstrap password
and recreate the API container. The account remains in PostgreSQL, while its
bootstrap secret no longer remains in runtime configuration.

```bash
sed -i '/^ADMIN_BOOTSTRAP_PASSWORD=/d' .env
docker compose --env-file .env -f deploy/compose.production.yml up -d --force-recreate api
```

## 5. Nginx and TLS

Copy the template, replace its domain, validate it, then enable it:

```bash
sudo cp deploy/nginx/captchaflow.conf.example /etc/nginx/sites-available/captchaflow
sudoedit /etc/nginx/sites-available/captchaflow
sudo ln -s /etc/nginx/sites-available/captchaflow /etc/nginx/sites-enabled/captchaflow
sudo nginx -t
sudo systemctl reload nginx
```

Once DNS and HTTP point to this server, issue and test the certificate:

```bash
sudo certbot --nginx -d captcha.example.com --redirect
sudo certbot renew --dry-run
curl --fail --silent --show-error https://captcha.example.com/healthz
curl --fail --silent --show-error https://captcha.example.com/openapi.json -o /tmp/captchaflow-openapi.json
```

## 6. First Operational Check

1. Sign in to the administrator console through the HTTPS domain.
2. Create a test CDK, activate it in a separate browser session, and create a user API key.
3. Send one controlled solve request with a unique `Idempotency-Key`.
4. Confirm it appears once in the user and administrator logs and quota is confirmed or refunded.
5. Confirm the solver health page exposes only sanitized health information.

Never put user API keys, CDKs, internal service keys, downstream responses, or
database dumps in Git, Nginx files, or tickets.

## 7. Updates, Rollback, and Backups

Before an update, record the current commit and create a PostgreSQL backup:

```bash
cd /opt/captchaflow
git rev-parse HEAD
mkdir -p backups
docker compose --env-file .env -f deploy/compose.production.yml exec -T postgres \
  pg_dump -U captchaflow -d captchaflow | gzip > "backups/captchaflow-$(date +%F-%H%M%S).sql.gz"
```

Deploy a reviewed commit or tag, rebuild, and verify public health:

```bash
git fetch --tags origin
git checkout <reviewed-tag-or-commit>
docker compose --env-file .env -f deploy/compose.production.yml up -d --build
curl --fail --silent --show-error https://captcha.example.com/healthz
```

To roll back application code, check out the previous known-good commit and
rerun the same Compose command. Database migrations are forward-only, so take
the backup before updates and restore data only as a deliberate recovery step.

Back up PostgreSQL daily to storage outside the host and test restores on a
separate machine. `docker compose down` keeps named volumes; do not run
`docker compose down -v` in production unless data destruction is intentional.
