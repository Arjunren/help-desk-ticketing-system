package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/Arjunren/help-desk-ticketing-system/internal/domain"
	"github.com/Arjunren/help-desk-ticketing-system/internal/store"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"net/mail"
	"strings"
	"time"
)

var (
	ErrCredentials  = errors.New("invalid credentials")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrTransition   = errors.New("invalid status transition")
)

type ValidationError struct{ Fields map[string]string }

func (e ValidationError) Error() string { return "validation failed" }
func invalid(k, v string) error         { return ValidationError{map[string]string{k: v}} }

type Claims struct {
	Role domain.Role `json:"role"`
	jwt.RegisteredClaims
}
type Service struct {
	store  store.Store
	secret []byte
	expiry time.Duration
	dummy  []byte
	now    func() time.Time
}

func New(st store.Store, secret string, expiry time.Duration) *Service {
	dummy, _ := bcrypt.GenerateFromPassword([]byte("dummy-password"), bcrypt.DefaultCost)
	return &Service{st, []byte(secret), expiry, dummy, time.Now}
}
func (s *Service) CreateUser(ctx context.Context, name, email, password string, role domain.Role) (domain.User, error) {
	name = strings.TrimSpace(name)
	email = strings.ToLower(strings.TrimSpace(email))
	p, err := mail.ParseAddress(email)
	if len(name) < 2 || len(name) > 120 {
		return domain.User{}, invalid("name", "must contain 2 to 120 characters")
	}
	if err != nil || p.Address != email {
		return domain.User{}, invalid("email", "must be valid")
	}
	if len(password) < 12 || len(password) > 72 {
		return domain.User{}, invalid("password", "must contain 12 to 72 characters")
	}
	if role != domain.RoleAdmin && role != domain.RoleAgent && role != domain.RoleRequester {
		return domain.User{}, invalid("role", "is invalid")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return domain.User{}, err
	}
	return s.store.CreateUser(ctx, domain.User{Name: name, Email: email, PasswordHash: string(hash), Role: role})
}
func (s *Service) Login(ctx context.Context, email, password string) (string, domain.User, error) {
	u, err := s.store.UserByEmail(ctx, strings.TrimSpace(email))
	if err != nil {
		_ = bcrypt.CompareHashAndPassword(s.dummy, []byte(password))
		if !errors.Is(err, store.ErrNotFound) {
			return "", u, err
		}
		return "", u, ErrCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil || !u.IsActive {
		return "", domain.User{}, ErrCredentials
	}
	now := s.now().UTC()
	id := uuid.NewString()
	claims := Claims{u.Role, jwt.RegisteredClaims{Subject: fmt.Sprint(u.ID), ID: id, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(s.expiry))}}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
	if err != nil {
		return "", u, err
	}
	if err = s.store.CreateSession(ctx, id, u.ID, now.Add(s.expiry)); err != nil {
		return "", u, err
	}
	return token, u, nil
}
func (s *Service) Verify(ctx context.Context, raw string) (Claims, error) {
	var c Claims
	t, err := jwt.ParseWithClaims(raw, &c, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, ErrUnauthorized
		}
		return s.secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	if err != nil || !t.Valid {
		return Claims{}, ErrUnauthorized
	}
	var uid int64
	if _, err = fmt.Sscan(c.Subject, &uid); err != nil {
		return Claims{}, ErrUnauthorized
	}
	ok, err := s.store.SessionActive(ctx, c.ID, uid)
	if err != nil || !ok {
		return Claims{}, ErrUnauthorized
	}
	return c, nil
}
func (s *Service) Logout(ctx context.Context, c Claims) error {
	var uid int64
	_, _ = fmt.Sscan(c.Subject, &uid)
	return s.store.RevokeSession(ctx, c.ID, uid)
}
func (s *Service) CreateTicket(ctx context.Context, actor int64, subject, description string, priority domain.Priority) (domain.Ticket, error) {
	subject = strings.TrimSpace(subject)
	description = strings.TrimSpace(description)
	if len(subject) < 3 || len(subject) > 180 {
		return domain.Ticket{}, invalid("subject", "must contain 3 to 180 characters")
	}
	if len(description) < 10 || len(description) > 5000 {
		return domain.Ticket{}, invalid("description", "must contain 10 to 5000 characters")
	}
	if !validPriority(priority) {
		return domain.Ticket{}, invalid("priority", "must be LOW, MEDIUM, HIGH, or URGENT")
	}
	number := "HD-" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))[:10]
	return s.store.CreateTicket(ctx, domain.Ticket{Number: number, Subject: subject, Description: description, Priority: priority, RequesterID: actor})
}
func (s *Service) GetTicket(ctx context.Context, id, actor int64, role domain.Role) (domain.Ticket, error) {
	t, err := s.store.TicketByID(ctx, id)
	if err != nil {
		return t, err
	}
	if role == domain.RoleRequester && t.RequesterID != actor {
		return domain.Ticket{}, ErrForbidden
	}
	return t, nil
}
func (s *Service) ListTickets(ctx context.Context, p domain.ListParams) ([]domain.Ticket, domain.Page, error) {
	items, total, err := s.store.ListTickets(ctx, p)
	return items, domain.NewPage(p, total), err
}
func (s *Service) UpdateTicket(ctx context.Context, id int64, status domain.Status, priority domain.Priority, assigneeID *int64) (domain.Ticket, error) {
	current, err := s.store.TicketByID(ctx, id)
	if err != nil {
		return current, err
	}
	if !validPriority(priority) {
		return current, invalid("priority", "is invalid")
	}
	if status != current.Status && !transitionAllowed(current.Status, status) {
		return current, ErrTransition
	}
	if assigneeID != nil {
		u, err := s.store.UserByID(ctx, *assigneeID)
		if err != nil {
			return current, err
		}
		if !u.IsActive || (u.Role != domain.RoleAgent && u.Role != domain.RoleAdmin) {
			return current, invalid("assignee_id", "must reference an active agent or admin")
		}
	}
	current.Status = status
	current.Priority = priority
	current.AssigneeID = assigneeID
	return s.store.UpdateTicket(ctx, current)
}
func (s *Service) AddComment(ctx context.Context, ticketID, actor int64, role domain.Role, body string) (domain.Comment, error) {
	if _, err := s.GetTicket(ctx, ticketID, actor, role); err != nil {
		return domain.Comment{}, err
	}
	body = strings.TrimSpace(body)
	if len(body) < 1 || len(body) > 2000 {
		return domain.Comment{}, invalid("body", "must contain 1 to 2000 characters")
	}
	return s.store.CreateComment(ctx, domain.Comment{TicketID: ticketID, AuthorID: actor, Body: body})
}
func (s *Service) Comments(ctx context.Context, ticketID, actor int64, role domain.Role) ([]domain.Comment, error) {
	if _, err := s.GetTicket(ctx, ticketID, actor, role); err != nil {
		return nil, err
	}
	return s.store.ListComments(ctx, ticketID)
}
func (s *Service) Dashboard(ctx context.Context, actor int64, role domain.Role) (domain.Dashboard, error) {
	return s.store.Dashboard(ctx, actor, role)
}
func validPriority(p domain.Priority) bool {
	return p == domain.PriorityLow || p == domain.PriorityMedium || p == domain.PriorityHigh || p == domain.PriorityUrgent
}
func transitionAllowed(from, to domain.Status) bool {
	switch from {
	case domain.StatusOpen:
		return to == domain.StatusInProgress
	case domain.StatusInProgress:
		return to == domain.StatusResolved
	case domain.StatusResolved:
		return to == domain.StatusClosed || to == domain.StatusInProgress
	default:
		return false
	}
}
