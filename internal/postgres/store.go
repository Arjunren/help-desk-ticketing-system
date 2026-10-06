package postgres

import (
	"context"
	"errors"
	"fmt"
	"github.com/Arjunren/help-desk-ticketing-system/internal/domain"
	basestore "github.com/Arjunren/help-desk-ticketing-system/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
	"time"
)

type Store struct{ Pool *pgxpool.Pool }

func Open(ctx context.Context, url string) (*Store, error) {
	p, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err = p.Ping(ctx); err != nil {
		p.Close()
		return nil, err
	}
	return &Store{p}, nil
}
func (s *Store) Close() { s.Pool.Close() }
func mapErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return basestore.ErrNotFound
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) && (pe.Code == "23505" || pe.Code == "23503" || pe.Code == "23514") {
		return basestore.ErrConflict
	}
	return err
}

const userCols = `id,email,password_hash,name,role,is_active,created_at,updated_at`

func (s *Store) UserByEmail(ctx context.Context, email string) (domain.User, error) {
	var u domain.User
	err := s.Pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE LOWER(email)=LOWER($1)`, email).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.Role, &u.IsActive, &u.CreatedAt, &u.UpdatedAt)
	return u, mapErr(err)
}
func (s *Store) UserByID(ctx context.Context, id int64) (domain.User, error) {
	var u domain.User
	err := s.Pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id=$1`, id).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.Role, &u.IsActive, &u.CreatedAt, &u.UpdatedAt)
	return u, mapErr(err)
}
func (s *Store) CreateUser(ctx context.Context, u domain.User) (domain.User, error) {
	err := s.Pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,name,role)VALUES(LOWER($1),$2,$3,$4)RETURNING `+userCols, u.Email, u.PasswordHash, u.Name, u.Role).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.Role, &u.IsActive, &u.CreatedAt, &u.UpdatedAt)
	return u, mapErr(err)
}
func (s *Store) CreateSession(ctx context.Context, id string, uid int64, exp time.Time) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO sessions(id,user_id,expires_at)VALUES($1,$2,$3)`, id, uid, exp)
	return err
}
func (s *Store) SessionActive(ctx context.Context, id string, uid int64) (bool, error) {
	var ok bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.id=$1 AND s.user_id=$2 AND s.revoked_at IS NULL AND s.expires_at>NOW() AND u.is_active)`, id, uid).Scan(&ok)
	return ok, err
}
func (s *Store) RevokeSession(ctx context.Context, id string, uid int64) error {
	_, err := s.Pool.Exec(ctx, `UPDATE sessions SET revoked_at=NOW() WHERE id=$1 AND user_id=$2`, id, uid)
	return err
}
func scanTicket(r interface{ Scan(...any) error }) (domain.Ticket, error) {
	var t domain.Ticket
	err := r.Scan(&t.ID, &t.Number, &t.Subject, &t.Description, &t.Status, &t.Priority, &t.RequesterID, &t.RequesterName, &t.AssigneeID, &t.AssigneeName, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

const ticketSelect = `SELECT t.id,t.number,t.subject,t.description,t.status,t.priority,t.requester_id,r.name,t.assignee_id,COALESCE(a.name,''),t.created_at,t.updated_at FROM tickets t JOIN users r ON r.id=t.requester_id LEFT JOIN users a ON a.id=t.assignee_id`

func (s *Store) CreateTicket(ctx context.Context, t domain.Ticket) (domain.Ticket, error) {
	var id int64
	err := s.Pool.QueryRow(ctx, `INSERT INTO tickets(number,subject,description,priority,requester_id)VALUES($1,$2,$3,$4,$5)RETURNING id`, t.Number, t.Subject, t.Description, t.Priority, t.RequesterID).Scan(&id)
	if err != nil {
		return t, mapErr(err)
	}
	return s.TicketByID(ctx, id)
}
func (s *Store) TicketByID(ctx context.Context, id int64) (domain.Ticket, error) {
	t, err := scanTicket(s.Pool.QueryRow(ctx, ticketSelect+` WHERE t.id=$1`, id))
	return t, mapErr(err)
}
func (s *Store) ListTickets(ctx context.Context, p domain.ListParams) ([]domain.Ticket, int64, error) {
	clauses := []string{`(LOWER(t.subject) LIKE $1 OR LOWER(t.number) LIKE $1 OR LOWER(t.description) LIKE $1)`}
	args := []any{"%" + strings.ToLower(p.Search) + "%"}
	add := func(sql string, v any) {
		args = append(args, v)
		clauses = append(clauses, fmt.Sprintf(sql, len(args)))
	}
	if p.ActorRole == domain.RoleRequester {
		add(`t.requester_id=$%d`, p.ActorID)
	}
	if p.Status != "" {
		add(`t.status=$%d`, p.Status)
	}
	if p.Priority != "" {
		add(`t.priority=$%d`, p.Priority)
	}
	if p.AssigneeID > 0 {
		add(`t.assignee_id=$%d`, p.AssigneeID)
	}
	where := " WHERE " + strings.Join(clauses, " AND ")
	var total int64
	if err := s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM tickets t`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, p.Limit, (p.Page-1)*p.Limit)
	q := fmt.Sprintf(`%s%s ORDER BY t.updated_at DESC,t.id DESC LIMIT $%d OFFSET $%d`, ticketSelect, where, len(args)-1, len(args))
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []domain.Ticket{}
	for rows.Next() {
		t, err := scanTicket(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, t)
	}
	return items, total, rows.Err()
}
func (s *Store) UpdateTicket(ctx context.Context, t domain.Ticket) (domain.Ticket, error) {
	res, err := s.Pool.Exec(ctx, `UPDATE tickets SET status=$2,priority=$3,assignee_id=$4,updated_at=NOW() WHERE id=$1`, t.ID, t.Status, t.Priority, t.AssigneeID)
	if err != nil {
		return t, mapErr(err)
	}
	if res.RowsAffected() == 0 {
		return t, basestore.ErrNotFound
	}
	return s.TicketByID(ctx, t.ID)
}
func (s *Store) CreateComment(ctx context.Context, c domain.Comment) (domain.Comment, error) {
	err := s.Pool.QueryRow(ctx, `INSERT INTO comments(ticket_id,author_id,body)VALUES($1,$2,$3)RETURNING id,created_at`, c.TicketID, c.AuthorID, c.Body).Scan(&c.ID, &c.CreatedAt)
	return c, mapErr(err)
}
func (s *Store) ListComments(ctx context.Context, tid int64) ([]domain.Comment, error) {
	rows, err := s.Pool.Query(ctx, `SELECT c.id,c.ticket_id,c.author_id,u.name,c.body,c.created_at FROM comments c JOIN users u ON u.id=c.author_id WHERE c.ticket_id=$1 ORDER BY c.created_at,c.id`, tid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.Comment{}
	for rows.Next() {
		var c domain.Comment
		if err := rows.Scan(&c.ID, &c.TicketID, &c.AuthorID, &c.AuthorName, &c.Body, &c.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, c)
	}
	return items, rows.Err()
}
func (s *Store) Dashboard(ctx context.Context, uid int64, role domain.Role) (domain.Dashboard, error) {
	scope := ""
	args := []any{}
	if role == domain.RoleRequester {
		scope = " WHERE requester_id=$1"
		args = append(args, uid)
	}
	var d domain.Dashboard
	q := `SELECT COUNT(*) FILTER(WHERE status='OPEN'),COUNT(*) FILTER(WHERE status='IN_PROGRESS'),COUNT(*) FILTER(WHERE status='RESOLVED'),COUNT(*) FILTER(WHERE status='CLOSED'),COUNT(*) FILTER(WHERE assignee_id IS NULL),COUNT(*) FILTER(WHERE priority='URGENT') FROM tickets` + scope
	err := s.Pool.QueryRow(ctx, q, args...).Scan(&d.Open, &d.InProgress, &d.Resolved, &d.Closed, &d.Unassigned, &d.Urgent)
	return d, err
}
