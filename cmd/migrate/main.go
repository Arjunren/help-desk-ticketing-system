package main

import (
	"context"
	"github.com/Arjunren/help-desk-ticketing-system/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"log"
	"os"
)

func main() {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		log.Fatal("DATABASE_URL is required")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err = migrations.Apply(ctx, db); err != nil {
		log.Fatal(err)
	}
	log.Print("migrations applied")
}
