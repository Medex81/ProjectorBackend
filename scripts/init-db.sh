#!/bin/bash
# scripts/init-db.sh

set -e

echo "Initializing databases for ProjectorBackend"
echo "=========================================="

# Database configurations
declare -A databases=(
    ["identity"]="identity"
    ["registry"]="registry"
    ["game"]="game"
    ["mail"]="mail"
)

# PostgreSQL connection
PG_HOST=${PG_HOST:-localhost}
PG_PORT=${PG_PORT:-5432}
PG_USER=${PG_USER:-projector}
PG_PASSWORD=${PG_PASSWORD:-projector123}

# Create databases
for db in "${!databases[@]}"; do
    echo "Creating database: $db"
    PGPASSWORD=$PG_PASSWORD psql -h $PG_HOST -p $PG_PORT -U $PG_USER -d postgres -c "CREATE DATABASE $db;" 2>/dev/null || echo "Database $db already exists"
done

# Run migrations
echo -e "\nRunning migrations..."

# Identity provider migrations
if [ -d "services/identity-provider/migrations" ]; then
    echo "Running identity provider migrations..."
    goose -dir services/identity-provider/migrations postgres "postgres://$PG_USER:$PG_PASSWORD@$PG_HOST:$PG_PORT/identity?sslmode=disable" up
fi

# Service registry migrations
if [ -d "services/service-registry/migrations" ]; then
    echo "Running service registry migrations..."
    goose -dir services/service-registry/migrations postgres "postgres://$PG_USER:$PG_PASSWORD@$PG_HOST:$PG_PORT/registry?sslmode=disable" up
fi

echo -e "\nDatabase initialization complete!"

# Create Redis flush script
echo -e "\nFlushing Redis (if running)..."
if command -v redis-cli &> /dev/null; then
    redis-cli -h ${REDIS_HOST:-localhost} -p ${REDIS_PORT:-6379} FLUSHALL || echo "Redis not available"
fi

echo -e "\n✓ All databases initialized successfully!"