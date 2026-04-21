# PW-Style Live Class + Learning Platform

A complete Go Fiber v3 backend that combines live streaming, recorded lectures,
practice testing, AI doubt solving, subscriptions, and multi-language content
— modelled after pw.live's feature set.

## Features

### Streaming & classroom
- ✅ JWT auth with student / instructor / admin roles
- ✅ RTMP ingest + HLS playback via Nginx-RTMP
- ✅ Live stream lifecycle (create, start, end) with auto-start from RTMP callbacks
- ✅ Automatic recording capture to MinIO
- ✅ WebSocket live chat + REST fallback + chat history

### PW-style learning
- ✅ **Exam categories** (JEE/NEET/UPSC/School/…) seeded on first migration
- ✅ **Courses**, **batches**, **enrollments** with progress tracking
- ✅ **Subjects → Chapters → Topics** taxonomy
- ✅ **Lectures** (live + recorded) tied to streams / recordings
- ✅ **Study materials** (PDFs/notes) uploaded to MinIO with presigned downloads
- ✅ **Lecture view history** for resume-watching & progress
- ✅ **Tests** (DPPs, chapter, subject, mocks, PYQs) with auto-scoring

### AI + doubts
- ✅ **Doubt solving** with text or voice URL input
- ✅ **Claude API** integration for instant AI answers
- ✅ Instructor answers, answer acceptance, pending queue

### Analytics
- ✅ Aggregate user stats (avg score, time, best, watched seconds)
- ✅ **Weak-topic detection** via per-topic accuracy
- ✅ Difficulty-level breakdown + recent attempts summary

### Subscriptions & payments
- ✅ Pre-seeded Free / Monthly / Yearly plans
- ✅ **Razorpay** checkout + signature verification + webhooks

### Discovery & i18n
- ✅ Postgres **full-text search** across courses + lectures
- ✅ Multi-language content column + `Accept-Language` / `?lang=` middleware

### Offline / OTT
- ✅ **Video quality variants** registry (240p – 1080p)
- ✅ **Time-limited download tokens** for offline use

### Coaching app (student + instructor + admin)
- ✅ **Attendance** — auto-mark from join time / watched seconds, manual override,
    bulk marking, monthly report, subject-wise %, low-attendance alerts, CSV export,
    geo-location + QR-code check-in for offline centers
- ✅ **Assignments** — create/update/delete, file + text submission, grading with feedback
- ✅ **Notifications** — per-user in-app notifications + read state + unread count
- ✅ **Announcements** — batch/course/global, optional fan-out to enrolled students
- ✅ **Fees management** — fee structures, student fees, installment scheduling,
    pending/overdue alerts, revenue dashboard
- ✅ **Bookmarks** — timestamped lecture/material bookmarks
- ✅ **Admin dashboard** — aggregate stats, course approval workflow, CSV exports

### Production hardening
- ✅ Structured logging (`slog`, JSON output)
- ✅ Per-IP token-bucket rate limiting
- ✅ Request validation (`go-playground/validator`)
- ✅ Deep health checks (`/health/deep` pings Postgres, Redis, MinIO, Kafka)
- ✅ Graceful shutdown with configurable timeout
- ✅ Tuned pgx connection pool
- ✅ Optional TLS serving
- ✅ Startup secret validation

## Tech Stack

- **Go Fiber v3**: High-performance web framework
- **PostgreSQL + SQLC**: Type-safe database queries
- **Redis**: Session storage and caching
- **MinIO**: S3-compatible object storage
- **Kafka**: Event streaming
- **Nginx-RTMP**: Live streaming server
- **Swagger**: API documentation
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

5. **Generate Swagger documentation:**
```bash
make swagger
```

6. **Build and run:**
```bash
make build
./bin/server
```

Or run directly:
```bash
make run
```

The API will be available at `http://localhost:3000`

## API Documentation

Interactive Swagger documentation is available at:
```
http://localhost:3000/swagger/index.html
```

The Swagger UI provides:
- Complete API endpoint documentation
- Request/response schemas
- Try-it-out functionality
- Authentication support

To regenerate Swagger docs after making changes:
```bash
make swagger
```

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

### Run with Hot-Reload (Recommended)

Using Air for automatic reloading on code changes:

```bash
make dev
```

This will:
- Watch for file changes
- Automatically rebuild and restart the server
- Show build errors in real-time

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
