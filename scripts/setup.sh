#!/bin/bash

echo "Setting up Live Platform..."

# Copy .env if it doesn't exist
if [ ! -f .env ]; then
    echo "Creating .env file..."
    cp .env.example .env
fi

# Start Docker services
echo "Starting Docker services..."
docker-compose -f docker/docker-compose.yml up -d

# Wait for services to be ready
echo "Waiting for services to start..."
sleep 10

# Run migrations
echo "Running database migrations..."
migrate -path migrations -database "postgresql://postgres:postgres@localhost:5432/live_platform?sslmode=disable" up

# Generate SQLC code
echo "Generating SQLC code..."
sqlc generate

echo "Setup complete! Run 'make run' to start the server."
