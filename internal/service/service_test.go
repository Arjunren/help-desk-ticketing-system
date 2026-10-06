package service

import (
	"context"
	"errors"
	"github.com/Arjunren/help-desk-ticketing-system/internal/domain"
	"github.com/Arjunren/help-desk-ticketing-system/internal/store"
	"testing"
	"time"
)

type fake struct {
	store.Store
	ticket  domain.Ticket
	user    domain.User
	updated domain.Ticket
}

func (f *fake) TicketByID(context.Context, int64) (domain.Ticket, error) { return f.ticket, nil }
func (f *fake) UpdateTicket(_ context.Context, t domain.Ticket) (domain.Ticket, error) {
	f.updated = t
	return t, nil
}
func (f *fake) UserByID(context.Context, int64) (domain.User, error) { return f.user, nil }
func TestInvalidStatusTransition(t *testing.T) {
	f := &fake{ticket: domain.Ticket{ID: 1, Status: domain.StatusOpen, Priority: domain.PriorityMedium}}
	s := New(f, "0123456789abcdef0123456789abcdef", time.Minute)
	_, err := s.UpdateTicket(context.Background(), 1, domain.StatusClosed, domain.PriorityMedium, nil)
	if !errors.Is(err, ErrTransition) {
		t.Fatalf("expected transition error, got %v", err)
	}
}
func TestRequesterCannotReadAnotherTicket(t *testing.T) {
	f := &fake{ticket: domain.Ticket{ID: 1, RequesterID: 2}}
	s := New(f, "0123456789abcdef0123456789abcdef", time.Minute)
	_, err := s.GetTicket(context.Background(), 1, 3, domain.RoleRequester)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
}
func TestAssigneeMustBeAgent(t *testing.T) {
	id := int64(9)
	f := &fake{ticket: domain.Ticket{ID: 1, Status: domain.StatusOpen, Priority: domain.PriorityMedium}, user: domain.User{ID: 9, Role: domain.RoleRequester, IsActive: true}}
	s := New(f, "0123456789abcdef0123456789abcdef", time.Minute)
	_, err := s.UpdateTicket(context.Background(), 1, domain.StatusOpen, domain.PriorityMedium, &id)
	var v ValidationError
	if !errors.As(err, &v) {
		t.Fatalf("expected validation error, got %v", err)
	}
}
