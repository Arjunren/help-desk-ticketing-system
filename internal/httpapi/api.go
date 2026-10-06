package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Arjunren/help-desk-ticketing-system/internal/domain"
	"github.com/Arjunren/help-desk-ticketing-system/internal/service"
	"github.com/Arjunren/help-desk-ticketing-system/internal/store"
	"github.com/go-chi/chi/v5"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
)

type API struct {
	svc *service.Service
	log *slog.Logger
}

func New(s *service.Service, l *slog.Logger) *API { return &API{s, l} }

type key string

const claimsKey key = "claims"

func (a *API) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(a.recover)
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) { write(w, 200, map[string]string{"status": "ok"}) })
	r.Post("/api/auth/register", a.register)
	r.With(newLoginLimiter(10).middleware).Post("/api/auth/login", a.login)
	r.Group(func(r chi.Router) {
		r.Use(a.authenticate)
		r.Post("/api/auth/logout", a.logout)
		r.Get("/api/dashboard", a.dashboard)
		r.With(a.roles(domain.RoleAdmin)).Post("/api/users", a.createUser)
		r.Get("/api/tickets", a.listTickets)
		r.Post("/api/tickets", a.createTicket)
		r.Get("/api/tickets/{id}", a.getTicket)
		r.With(a.roles(domain.RoleAdmin, domain.RoleAgent)).Put("/api/tickets/{id}", a.updateTicket)
		r.Get("/api/tickets/{id}/comments", a.listComments)
		r.Post("/api/tickets/{id}/comments", a.addComment)
	})
	return r
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func apiError(w http.ResponseWriter, status int, code, msg string, fields map[string]string) {
	write(w, status, map[string]any{"error": map[string]any{"code": code, "message": msg, "fields": fields}})
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return errors.New("content type")
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err = d.Decode(v); err != nil {
		return err
	}
	if err = d.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("multiple values")
	}
	return nil
}
func (a *API) fail(w http.ResponseWriter, err error) {
	var v service.ValidationError
	switch {
	case errors.As(err, &v):
		apiError(w, 422, "VALIDATION_ERROR", "Invalid request data", v.Fields)
	case errors.Is(err, service.ErrCredentials):
		apiError(w, 401, "INVALID_CREDENTIALS", "Email or password is incorrect", nil)
	case errors.Is(err, service.ErrUnauthorized):
		apiError(w, 401, "UNAUTHORIZED", "A valid bearer token is required", nil)
	case errors.Is(err, service.ErrForbidden):
		apiError(w, 403, "FORBIDDEN", "You cannot access this ticket", nil)
	case errors.Is(err, service.ErrTransition):
		apiError(w, 409, "INVALID_STATUS_TRANSITION", "The requested status transition is not allowed", nil)
	case errors.Is(err, store.ErrNotFound):
		apiError(w, 404, "NOT_FOUND", "The requested resource was not found", nil)
	case errors.Is(err, store.ErrConflict):
		apiError(w, 409, "CONFLICT", "The request conflicts with existing data", nil)
	default:
		a.log.Error("request failed", "error", err)
		apiError(w, 500, "INTERNAL_ERROR", "An internal error occurred", nil)
	}
}
func (a *API) recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recover() != nil {
				a.log.Error("request panic", "path", r.URL.Path)
				apiError(w, 500, "INTERNAL_ERROR", "An internal error occurred", nil)
			}
		}()
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
func (a *API) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.SplitN(r.Header.Get("Authorization"), " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			apiError(w, 401, "UNAUTHORIZED", "A valid bearer token is required", nil)
			return
		}
		c, err := a.svc.Verify(r.Context(), parts[1])
		if err != nil {
			a.fail(w, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), claimsKey, c)))
	})
}
func (a *API) roles(roles ...domain.Role) func(http.Handler) http.Handler {
	allowed := map[domain.Role]bool{}
	for _, r := range roles {
		allowed[r] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := claims(r)
			if !allowed[c.Role] {
				apiError(w, 403, "FORBIDDEN", "Your role cannot perform this action", nil)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
func claims(r *http.Request) service.Claims {
	c, _ := r.Context().Value(claimsKey).(service.Claims)
	return c
}
func actor(r *http.Request) int64 { var id int64; _, _ = fmt.Sscan(claims(r).Subject, &id); return id }
func idParam(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		apiError(w, 400, "INVALID_ID", "ID must be a positive integer", nil)
		return 0, false
	}
	return id, true
}

type userInput struct {
	Name     string      `json:"name"`
	Email    string      `json:"email"`
	Password string      `json:"password"`
	Role     domain.Role `json:"role"`
}

func (a *API) register(w http.ResponseWriter, r *http.Request) {
	var in userInput
	if decode(w, r, &in) != nil {
		apiError(w, 400, "INVALID_JSON", "Request body must be valid JSON", nil)
		return
	}
	u, err := a.svc.CreateUser(r.Context(), in.Name, in.Email, in.Password, domain.RoleRequester)
	if err != nil {
		a.fail(w, err)
		return
	}
	write(w, 201, map[string]any{"data": u})
}
func (a *API) createUser(w http.ResponseWriter, r *http.Request) {
	var in userInput
	if decode(w, r, &in) != nil {
		apiError(w, 400, "INVALID_JSON", "Request body must be valid JSON", nil)
		return
	}
	u, err := a.svc.CreateUser(r.Context(), in.Name, in.Email, in.Password, in.Role)
	if err != nil {
		a.fail(w, err)
		return
	}
	write(w, 201, map[string]any{"data": u})
}
func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if decode(w, r, &in) != nil {
		apiError(w, 400, "INVALID_JSON", "Request body must be valid JSON", nil)
		return
	}
	token, u, err := a.svc.Login(r.Context(), in.Email, in.Password)
	if err != nil {
		a.fail(w, err)
		return
	}
	write(w, 200, map[string]any{"data": map[string]any{"access_token": token, "token_type": "Bearer", "user": u}})
}
func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	if err := a.svc.Logout(r.Context(), claims(r)); err != nil {
		a.fail(w, err)
		return
	}
	w.WriteHeader(204)
}
func (a *API) createTicket(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Subject     string          `json:"subject"`
		Description string          `json:"description"`
		Priority    domain.Priority `json:"priority"`
	}
	if decode(w, r, &in) != nil {
		apiError(w, 400, "INVALID_JSON", "Request body must be valid JSON", nil)
		return
	}
	t, err := a.svc.CreateTicket(r.Context(), actor(r), in.Subject, in.Description, in.Priority)
	if err != nil {
		a.fail(w, err)
		return
	}
	write(w, 201, map[string]any{"data": t})
}
func (a *API) getTicket(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	t, err := a.svc.GetTicket(r.Context(), id, actor(r), claims(r).Role)
	if err != nil {
		a.fail(w, err)
		return
	}
	write(w, 200, map[string]any{"data": t})
}
func (a *API) listTickets(w http.ResponseWriter, r *http.Request) {
	p := domain.ListParams{Page: 1, Limit: 20, Search: strings.TrimSpace(r.URL.Query().Get("search")), Status: domain.Status(strings.ToUpper(r.URL.Query().Get("status"))), Priority: domain.Priority(strings.ToUpper(r.URL.Query().Get("priority"))), ActorID: actor(r), ActorRole: claims(r).Role}
	var err error
	if v := r.URL.Query().Get("page"); v != "" {
		p.Page, err = strconv.Atoi(v)
		if err != nil || p.Page < 1 {
			apiError(w, 400, "INVALID_QUERY", "page must be positive", nil)
			return
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		p.Limit, err = strconv.Atoi(v)
		if err != nil || p.Limit < 1 || p.Limit > 100 {
			apiError(w, 400, "INVALID_QUERY", "limit must be 1 to 100", nil)
			return
		}
	}
	if v := r.URL.Query().Get("assignee_id"); v != "" {
		p.AssigneeID, err = strconv.ParseInt(v, 10, 64)
		if err != nil || p.AssigneeID < 1 {
			apiError(w, 400, "INVALID_QUERY", "assignee_id must be positive", nil)
			return
		}
	}
	items, page, err := a.svc.ListTickets(r.Context(), p)
	if err != nil {
		a.fail(w, err)
		return
	}
	write(w, 200, map[string]any{"data": items, "meta": page})
}
func (a *API) updateTicket(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var in struct {
		Status     domain.Status   `json:"status"`
		Priority   domain.Priority `json:"priority"`
		AssigneeID *int64          `json:"assignee_id"`
	}
	if decode(w, r, &in) != nil {
		apiError(w, 400, "INVALID_JSON", "Request body must be valid JSON", nil)
		return
	}
	t, err := a.svc.UpdateTicket(r.Context(), id, in.Status, in.Priority, in.AssigneeID)
	if err != nil {
		a.fail(w, err)
		return
	}
	write(w, 200, map[string]any{"data": t})
}
func (a *API) addComment(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var in struct {
		Body string `json:"body"`
	}
	if decode(w, r, &in) != nil {
		apiError(w, 400, "INVALID_JSON", "Request body must be valid JSON", nil)
		return
	}
	c, err := a.svc.AddComment(r.Context(), id, actor(r), claims(r).Role, in.Body)
	if err != nil {
		a.fail(w, err)
		return
	}
	write(w, 201, map[string]any{"data": c})
}
func (a *API) listComments(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	items, err := a.svc.Comments(r.Context(), id, actor(r), claims(r).Role)
	if err != nil {
		a.fail(w, err)
		return
	}
	write(w, 200, map[string]any{"data": items})
}
func (a *API) dashboard(w http.ResponseWriter, r *http.Request) {
	d, err := a.svc.Dashboard(r.Context(), actor(r), claims(r).Role)
	if err != nil {
		a.fail(w, err)
		return
	}
	write(w, 200, map[string]any{"data": d})
}
