# Live Class Streaming Platform

A complete Go Fiber v3 backend for a live class streaming platform with PostgreSQL, Redis, MinIO, Kafka, and Nginx-RTMP.

## Features

- ✅ **User Authentication**: JWT-based auth with access & refresh tokens
- ✅ **Role-Based Access**: Student, Instructor, and Admin roles
- ✅ **Live Streaming**: RTMP ingest with HLS playback
- ✅ **Stream Management**: Create, start, end, and monitor streams
- ✅ **Video Recording**: Automatic recording storage in MinIO
- ✅ **Real-time Chat**: Live chat during streams
- ✅ **Event Streaming**: Kafka for async event processing
- ✅ **Caching**: Redis for session management

## Tech Stack

- **Go Fiber v3**: High-performance web framework
- **PostgreSQL + SQLC**: Type-safe database queries
- **Redis**: Session storage and caching
- **MinIO**: S3-compatible object storage
- **Kafka**: Event streaming
- **Nginx-RTMP**: Live streaming server
- **Docker**: Containerized deployment

## Project Structure

```
.
├── cmd/server/              # Application entry point
├── internal/
│   ├── auth/               # Authentication (JWT, login, register)
│   ├── users/              # User management
│   ├── stream/             # Stream management
│   ├── chat/               # Chat functionality
│   ├── recording/          # Recording management
│   ├── storage/            # MinIO client
│   ├── events/             # Kafka producer/consumer
│   ├── database/           # DB connections & generated code
│   ├── middleware/         # HTTP middleware
│   ├── config/             # Configuration
│   └── utils/              # Utilities (JWT, password hashing)
├── sql/
│   ├── queries/            # SQLC query definitions
│   └── schema/             # Database schemas
├── migrations/             # SQL migrations
├── docker-compose.yml      # Docker services
├── Dockerfile              # App container
└── Makefile                # Build commands
```

## Quick Start

### Prerequisites

- Go 1.22+
- Docker & Docker Compose
- SQLC CLI: `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`
- Migrate CLI: `go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest`

### Installation

1. **Install dependencies:**
```bash
make install
```

2. **Start all services with Docker:**
```bash
make docker-up
```

3. **Wait for services to be ready (30 seconds), then run migrations:**
```bash
./scripts/migrate.sh
```

4. **Generate SQLC code:**
```bash
make sqlc
```

5. **Build and run:**
```bash
make build
./bin/server
```

Or run directly:
```bash
make run
```

The API will be available at `http://localhost:3000`

## API Endpoints

### Authentication

#### Register Student
```http
POST /api/v1/auth/register/student
Content-Type: application/json

{
  "email": "student@example.com",
  "username": "student1",
  "password": "password123",
  "full_name": "John Student"
}
```

#### Register Instructor
```http
POST /api/v1/auth/register/instructor
Content-Type: application/json

{
  "email": "instructor@example.com",
  "username": "instructor1",
  "password": "password123",
  "full_name": "Jane Instructor"
}
```

#### Register Admin
```http
POST /api/v1/auth/register/admin
Content-Type: application/json

{
  "email": "admin@example.com",
  "username": "admin1",
  "password": "password123",
  "full_name": "Admin User"
}
```

#### Login (All Roles)
```http
POST /api/v1/auth/login
Content-Type: application/json

{
  "email": "student@example.com",
  "password": "password123"
}

Response:
{
  "access_token": "eyJhbGc...",
  "refresh_token": "eyJhbGc...",
  "user": {
    "id": "uuid",
    "email": "student@example.com",
    "username": "student1",
    "full_name": "John Student",
    "role": "student"
  }
}
```

#### Refresh Token
```http
POST /api/v1/auth/refresh
Content-Type: application/json

{
  "refresh_token": "eyJhbGc..."
}
```

#### Logout
```http
POST /api/v1/auth/logout
Authorization: Bearer {access_token}
```

### User Management

#### Get Profile
```http
GET /api/v1/users/profile
Authorization: Bearer {access_token}
```

#### Update Profile
```http
PUT /api/v1/users/profile
Authorization: Bearer {access_token}
Content-Type: application/json

{
  "full_name": "Updated Name"
}
```

#### List Users (Admin Only)
```http
GET /api/v1/users?limit=10&offset=0
Authorization: Bearer {access_token}
```

### Stream Management

#### List Live Streams (Public)
```http
GET /api/v1/streams/live
```

#### Get Stream Details (Public)
```http
GET /api/v1/streams/{stream_id}
```

#### Create Stream (Instructor/Admin Only)
```http
POST /api/v1/streams
Authorization: Bearer {access_token}
Content-Type: application/json

{
  "title": "Introduction to Go Programming",
  "description": "Learn Go basics",
  "scheduled_at": "2025-12-01T10:00:00Z"
}

Response:
{
  "id": "uuid",
  "title": "Introduction to Go Programming",
  "stream_key": "unique-stream-key",
  "status": "scheduled",
  ...
}
```

#### Start Stream (Instructor/Admin Only)
```http
POST /api/v1/streams/{stream_id}/start
Authorization: Bearer {access_token}
```

#### End Stream (Instructor/Admin Only)
```http
POST /api/v1/streams/{stream_id}/end
Authorization: Bearer {access_token}
```

### Recordings

#### Get Recording
```http
GET /api/v1/recordings/{recording_id}
Authorization: Bearer {access_token}
```

#### Get Recording Playback URL
```http
GET /api/v1/recordings/{recording_id}/url
Authorization: Bearer {access_token}

Response:
{
  "url": "https://minio:9000/recordings/file.mp4?..."
}
```

#### Get Recordings by Stream
```http
GET /api/v1/recordings/stream/{stream_id}
Authorization: Bearer {access_token}
```

### Chat

#### Send Chat Message
```http
POST /api/v1/chat/{stream_id}
Authorization: Bearer {access_token}
Content-Type: application/json

{
  "message": "Hello everyone!"
}
```

## RTMP Streaming

### For Instructors

After creating a stream, use the `stream_key` to broadcast:

```bash
# Using OBS Studio or FFmpeg
ffmpeg -re -i input.mp4 -c:v libx264 -c:a aac \
  -f flv rtmp://localhost:1935/live/{stream_key}
```

### For Students

Watch the HLS stream:

```
http://localhost:8080/hls/{stream_key}.m3u8
```

## Docker Services

The `docker-compose.yml` includes:

- **PostgreSQL** (port 5432): Database
- **Redis** (port 6379): Cache & sessions
- **MinIO** (ports 9000, 9001): Object storage
- **Kafka** (port 9092): Event streaming
- **Nginx-RTMP** (ports 1935, 8080): Streaming server
- **App** (port 3000): Go Fiber API

### Docker Commands

```bash
# Start all services
make docker-up

# Stop all services
make docker-down

# View logs
make docker-logs

# Rebuild containers
make docker-build
```

## Development

### Run Locally (without Docker)

1. Start infrastructure services:
```bash
make docker-up
```

2. Run migrations:
```bash
make migrate-up
```

3. Generate SQLC:
```bash
make sqlc
```

4. Run the server:
```bash
make run
```

### Database Migrations

```bash
# Run migrations
make migrate-up

# Rollback migrations
make migrate-down
```

### Generate SQLC Code

After modifying SQL queries or schemas:

```bash
make sqlc
```

## Environment Variables

Copy `.env.example` to `.env` and configure:

```env
SERVER_PORT=3000
ENV=development

# Database
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=postgres
DB_NAME=live_platform

# Redis
REDIS_HOST=localhost
REDIS_PORT=6379

# Kafka
KAFKA_BROKER=localhost:9092

# MinIO
MINIO_ENDPOINT=localhost:9000
MINIO_ACCESS_KEY=minioadmin
MINIO_SECRET_KEY=minioadmin

# JWT
JWT_ACCESS_SECRET=your-secret-key
JWT_REFRESH_SECRET=your-refresh-secret
JWT_ACCESS_EXPIRY=15m
JWT_REFRESH_EXPIRY=7d
```

## Testing with cURL

### Complete Flow Example

```bash
# 1. Register an instructor
curl -X POST http://localhost:3000/api/v1/auth/register/instructor \
  -H "Content-Type: application/json" \
  -d '{"email":"teacher@test.com","username":"teacher1","password":"pass123","full_name":"Test Teacher"}'

# 2. Login
TOKEN=$(curl -X POST http://localhost:3000/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"teacher@test.com","password":"pass123"}' | jq -r '.access_token')

# 3. Create a stream
STREAM=$(curl -X POST http://localhost:3000/api/v1/streams \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"title":"My First Stream","description":"Test stream","scheduled_at":"2025-12-01T10:00:00Z"}')

STREAM_ID=$(echo $STREAM | jq -r '.id')
STREAM_KEY=$(echo $STREAM | jq -r '.stream_key')

# 4. Start the stream
curl -X POST http://localhost:3000/api/v1/streams/$STREAM_ID/start \
  -H "Authorization: Bearer $TOKEN"

# 5. Start broadcasting (use OBS or FFmpeg with the stream_key)
# rtmp://localhost:1935/live/$STREAM_KEY

# 6. List live streams
curl http://localhost:3000/api/v1/streams/live
```

## Architecture Decisions

- **Modular Monolith**: Single repository with clear module boundaries
- **Clean Architecture**: Separation of handlers, services, and repositories
- **Type-Safe Queries**: SQLC generates Go code from SQL
- **Dependency Injection**: Services injected into handlers
- **Event-Driven**: Kafka for async processing
- **Stateless API**: JWT tokens, no server-side sessions

## Production Considerations

- [ ] Use proper secrets management (AWS Secrets Manager, Vault)
- [ ] Enable HTTPS/TLS
- [ ] Add rate limiting
- [ ] Implement proper logging (structured logs)
- [ ] Add monitoring (Prometheus, Grafana)
- [ ] Set up CI/CD pipeline
- [ ] Configure database connection pooling
- [ ] Add request validation middleware
- [ ] Implement graceful shutdown
- [ ] Add health checks for all services

## License

MIT

## Support

For issues and questions, please open a GitHub issue.
