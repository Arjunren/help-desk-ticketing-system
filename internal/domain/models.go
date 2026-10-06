package domain

import "time"

type Role string

const (
	RoleAdmin     Role = "ADMIN"
	RoleAgent     Role = "AGENT"
	RoleRequester Role = "REQUESTER"
)

type Status string

const (
	StatusOpen       Status = "OPEN"
	StatusInProgress Status = "IN_PROGRESS"
	StatusResolved   Status = "RESOLVED"
	StatusClosed     Status = "CLOSED"
)

type Priority string

const (
	PriorityLow    Priority = "LOW"
	PriorityMedium Priority = "MEDIUM"
	PriorityHigh   Priority = "HIGH"
	PriorityUrgent Priority = "URGENT"
)

type User struct {
	ID           int64     `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Name         string    `json:"name"`
	Role         Role      `json:"role"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
type Ticket struct {
	ID            int64     `json:"id"`
	Number        string    `json:"number"`
	Subject       string    `json:"subject"`
	Description   string    `json:"description"`
	Status        Status    `json:"status"`
	Priority      Priority  `json:"priority"`
	RequesterID   int64     `json:"requester_id"`
	RequesterName string    `json:"requester_name,omitempty"`
	AssigneeID    *int64    `json:"assignee_id"`
	AssigneeName  string    `json:"assignee_name,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
type Comment struct {
	ID         int64     `json:"id"`
	TicketID   int64     `json:"ticket_id"`
	AuthorID   int64     `json:"author_id"`
	AuthorName string    `json:"author_name,omitempty"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
}
type Dashboard struct {
	Open       int64 `json:"open"`
	InProgress int64 `json:"in_progress"`
	Resolved   int64 `json:"resolved"`
	Closed     int64 `json:"closed"`
	Unassigned int64 `json:"unassigned"`
	Urgent     int64 `json:"urgent"`
}
type ListParams struct {
	Page, Limit int
	Search      string
	Status      Status
	Priority    Priority
	AssigneeID  int64
	ActorID     int64
	ActorRole   Role
}
type Page struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

func NewPage(p ListParams, total int64) Page {
	pages := 0
	if total > 0 {
		pages = int((total + int64(p.Limit) - 1) / int64(p.Limit))
	}
	return Page{p.Page, p.Limit, total, pages}
}
