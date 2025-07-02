// Copyright 2025 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Tool devpostdash is a devpost dashboard scraper.
package main

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/goccy/go-yaml"
)

func printProjects(projects []Project) {
	for _, p := range projects {
		fmt.Printf("%s: %s\n", p.Title, p.URL)
		// ID, URL, Image, Description, Team
		suffix := ""
		if p.Winner {
			suffix = " (winner)"
		}
		fmt.Printf("- %s %s%s\n", p.Title, p.Tagline, suffix)
		fmt.Printf("  %s\n", p.Team)
	}
}

func runWebserver(ctx context.Context, projects []Project) error {
	tmpl, err := template.ParseFiles("templates/index.html")
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if err := tmpl.Execute(w, projects); err != nil {
			log.Printf("Error executing template: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	port := ":8080"
	log.Printf("Starting server on port %s", port)

	server := &http.Server{Addr: port}
	errCh := make(chan error)
	go func() {
		err2 := server.ListenAndServe()
		if errors.Is(err2, http.ErrServerClosed) {
			err2 = nil
		}
		errCh <- err2
	}()

	select {
	case <-ctx.Done():
		log.Printf("Shutting down ...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := server.Shutdown(shutdownCtx)
		shutdownCancel()
		if err != nil {
			return fmt.Errorf("server shutdown failed: %w", err)
		}
		return <-errCh
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("server error: %w", err)
		}
	}
	return nil
}

func mainImpl() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	config, err := os.ReadFile("config.yml")
	if err != nil {
		return err
	}
	var c Config
	if err := yaml.Unmarshal(config, &c); err != nil {
		return err
	}
	projects, err := scrapeDevpost(ctx, c)
	if err != nil {
		return err
	}
	if len(projects) == 0 {
		return errors.New("no projects found")
	}
	log.Printf("Found %d projects", len(projects))
	printProjects(projects)
	return runWebserver(ctx, projects)
}

func main() {
	if err := mainImpl(); err != nil {
		if err != context.Canceled {
			fmt.Fprintln(os.Stderr, "devpostdash:", err)
			os.Exit(1)
		}
	}
}
