#!/usr/bin/env bash
set -euxo pipefail

cd "$(dirname "$0")/.."

echo "Pulling latest changes..."
git pull --ff-only

echo "Building images..."
docker compose build

echo "Restarting services..."
docker compose up -d

echo "Cleaning up old images..."
docker image prune -f

echo "Done. Checking status..."
docker compose ps
