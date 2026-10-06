CREATE TABLE users (
  id BIGSERIAL PRIMARY KEY,
  email VARCHAR(255) NOT NULL,
  password_hash VARCHAR(255) NOT NULL,
  name VARCHAR(120) NOT NULL,
  role VARCHAR(20) NOT NULL CHECK (role IN ('ADMIN','AGENT','REQUESTER')),
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX users_email_unique_ci ON users(LOWER(email));
CREATE TABLE sessions (
  id UUID PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TIMESTAMPTZ NOT NULL,
  revoked_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX sessions_user_idx ON sessions(user_id);
CREATE TABLE tickets (
  id BIGSERIAL PRIMARY KEY,
  number VARCHAR(24) NOT NULL UNIQUE,
  subject VARCHAR(180) NOT NULL,
  description TEXT NOT NULL,
  status VARCHAR(20) NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN','IN_PROGRESS','RESOLVED','CLOSED')),
  priority VARCHAR(20) NOT NULL CHECK (priority IN ('LOW','MEDIUM','HIGH','URGENT')),
  requester_id BIGINT NOT NULL REFERENCES users(id),
  assignee_id BIGINT REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX tickets_requester_idx ON tickets(requester_id);
CREATE INDEX tickets_assignee_idx ON tickets(assignee_id);
CREATE INDEX tickets_status_priority_idx ON tickets(status,priority);
CREATE TABLE comments (
  id BIGSERIAL PRIMARY KEY,
  ticket_id BIGINT NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
  author_id BIGINT NOT NULL REFERENCES users(id),
  body TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX comments_ticket_idx ON comments(ticket_id,created_at);
