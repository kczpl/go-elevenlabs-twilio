package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"caller/internal/config"
	"caller/internal/server"
)

func main() {
	if err := run(); err != nil {
		log.Printf("error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	srv, err := server.New(cfg)
	if err != nil {
		return err
	}

	go func() {
		if err := srv.Start(); err != nil {
			log.Printf("server error: %v\n", err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}

	return nil
}
