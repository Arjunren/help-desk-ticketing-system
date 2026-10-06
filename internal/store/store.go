package store

import (
	"context"
	"errors"
	"github.com/Arjunren/help-desk-ticketing-system/internal/domain"
	"time"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

type Store interface {
	UserByEmail(context.Context, string) (domain.User, error)
	UserByID(context.Context, int64) (domain.User, error)
	CreateUser(context.Context, domain.User) (domain.User, error)
	CreateSession(context.Context, string, int64, time.Time) error
	SessionActive(context.Context, string, int64) (bool, error)
	RevokeSession(context.Context, string, int64) error
	CreateTicket(context.Context, domain.Ticket) (domain.Ticket, error)
	TicketByID(context.Context, int64) (domain.Ticket, error)
	ListTickets(context.Context, domain.ListParams) ([]domain.Ticket, int64, error)
	UpdateTicket(context.Context, domain.Ticket) (domain.Ticket, error)
	CreateComment(context.Context, domain.Comment) (domain.Comment, error)
	ListComments(context.Context, int64) ([]domain.Comment, error)
	Dashboard(context.Context, int64, domain.Role) (domain.Dashboard, error)
}
