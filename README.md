# Help Desk Ticketing System

## Overview

A small, secure Go REST API for submitting, assigning, discussing, and resolving support tickets. Requesters see only their own tickets; agents and administrators can work across the queue.

## Features

- Requester registration, login, logout, and revocable JWT sessions
- Admin, agent, and requester roles
- Ticket creation, assignment, priority, controlled status transitions, and comments
- Object-level ownership checks for requester ticket and comment access
- Search and filters for status, priority, and assignee with pagination
- Role-aware dashboard counts
- PostgreSQL migrations, development seeding, Docker support, tests, and OpenAPI documentation

## Technology Stack

Go 1.25+, Chi, pgx, PostgreSQL 14+, bcrypt, JWT, Docker Compose.

## Requirements

- Go 1.25 or newer
- PostgreSQL 14 or newer
- Docker with Compose (optional)

## Installation

```bash
git clone https://github.com/Arjunren/help-desk-ticketing-system.git
cd help-desk-ticketing-system
go mod download
cp .env.example .env
```

PowerShell uses `Copy-Item .env.example .env`.

## Environment Variables

`APP_ADDR`, `DATABASE_URL`, `JWT_SECRET` (at least 32 characters), `JWT_EXPIRY` (5 minutes to 24 hours), `ALLOWED_ORIGINS`, and the four `SEED_*` values documented in `.env.example` are required as applicable. Samples are local-only and contain no production secret.

To load `.env` into the current PowerShell session:

```powershell
Get-Content .env | Where-Object { $_ -match '^[^#].+=' } | ForEach-Object {
  $name, $value = $_ -split '=', 2
  Set-Item -Path "Env:$name" -Value $value
}
```

## Database Setup

```bash
go run ./cmd/migrate
go run ./cmd/seed
```

Migrations are embedded in the migration binary and recorded in `schema_migrations`. The seeder is idempotent for its development accounts.

## Running the Application

```bash
go run ./cmd/api
```

Docker workflow:

```bash
docker compose up -d db
docker compose --profile tools run --rm migrate
docker compose --profile tools run --rm seed
docker compose up -d api
```

The service listens on `http://localhost:8080`; `GET /health` is public.

## Running Tests

```bash
go test ./...
go vet ./...
```

CI also runs all tests with the race detector and builds the API, migration, and seed commands.

## Default Development Accounts

- Admin: `admin@example.com` / `AdminPass123!`
- Agent: `agent@example.com` / `AgentPass123!`

Public registration always creates a requester account. Change all seeded passwords outside an isolated local environment.

## API Endpoints

| Method | Route | Access |
|---|---|---|
| POST | `/api/auth/register` | Public requester registration |
| POST | `/api/auth/login` | Public |
| POST | `/api/auth/logout` | Authenticated |
| GET | `/api/dashboard` | Authenticated, role-scoped |
| POST | `/api/users` | Admin |
| GET, POST | `/api/tickets` | Authenticated |
| GET | `/api/tickets/{id}` | Authenticated with ownership check |
| PUT | `/api/tickets/{id}` | Admin or agent |
| GET, POST | `/api/tickets/{id}/comments` | Authenticated with ownership check |

Collection query parameters include `page`, `limit` (maximum 100), `search`, `status`, `priority`, and `assignee_id`. See `docs/openapi.yaml`.

## Folder Structure

```text
cmd/                 API, migration, and seed entry points
internal/config/     Environment validation
internal/domain/     Domain models
internal/httpapi/    HTTP transport and authorization middleware
internal/postgres/   Parameterized PostgreSQL repository
internal/service/    Authentication, validation, ownership, and transition rules
internal/store/      Repository contract
migrations/          Embedded SQL migrations
docs/                OpenAPI description
```

## Security Notes

Passwords use bcrypt and are never serialized. JWTs require expiry and a live database session, so logout and account deactivation take effect server-side. Requester ownership is checked before ticket or comment access. Role checks are enforced on the server. Request DTOs reject unknown fields and oversized bodies. SQL values are parameterized, CORS uses an explicit allowlist, login attempts are rate-limited per process, responses omit stack traces, and security headers disable sniffing and framing.

## Known Limitations

- API-only; no email notifications or attachment uploads.
- Ticket transitions use one simple workflow rather than configurable service-level agreements.
- The system records ticket comments and timestamps but does not provide a separate immutable change-event table.
