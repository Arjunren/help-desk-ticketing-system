package main

import (
	"context"
	"errors"
	"github.com/Arjunren/help-desk-ticketing-system/internal/domain"
	"github.com/Arjunren/help-desk-ticketing-system/internal/postgres"
	"github.com/Arjunren/help-desk-ticketing-system/internal/service"
	"github.com/Arjunren/help-desk-ticketing-system/internal/store"
	"log"
	"os"
)

func main() {
	ctx := context.Background()
	db, err := postgres.Open(ctx, need("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	svc := service.New(db, "seed-only-secret-not-used-for-tokens", 0)
	seed(ctx, svc, db, need("SEED_ADMIN_EMAIL"), need("SEED_ADMIN_PASSWORD"), "Development Admin", domain.RoleAdmin)
	seed(ctx, svc, db, need("SEED_AGENT_EMAIL"), need("SEED_AGENT_PASSWORD"), "Development Agent", domain.RoleAgent)
}
func seed(ctx context.Context, svc *service.Service, db store.Store, email, password, name string, role domain.Role) {
	if _, err := db.UserByEmail(ctx, email); err == nil {
		log.Printf("%s exists; skipped", email)
		return
	} else if !errors.Is(err, store.ErrNotFound) {
		log.Fatal(err)
	}
	if _, err := svc.CreateUser(ctx, name, email, password, role); err != nil {
		log.Fatal(err)
	}
}
func need(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("%s is required", k)
	}
	return v
}
