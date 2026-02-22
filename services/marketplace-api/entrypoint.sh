#!/bin/sh
set -e
./goose -dir /app/migrations postgres "$DATABASE_URL" up
exec ./server
