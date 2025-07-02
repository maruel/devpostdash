// Copyright 2025 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"time"
)

//go:embed templates/*.html
var templatesFS embed.FS

var templates = template.Must(template.ParseFS(templatesFS, "templates/*.html"))

func runWebserver(ctx context.Context, host, title string, projects []Project) error {
	tmpl := templates.Lookup("cards.html")
	if tmpl == nil {
		return fmt.Errorf("failed to parse template cards.html")
	}

	mux := http.ServeMux{}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data := map[string]any{
			"Title":    title,
			"Projects": projects,
		}
		if err := tmpl.Execute(w, data); err != nil {
			slog.ErrorContext(ctx, "web", "err", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", host)
	if err != nil {
		return err
	}
	slog.InfoContext(ctx, "web", "listening", ln.Addr())
	s := &http.Server{Handler: &mux}
	errCh := make(chan error)
	go func() {
		err2 := s.Serve(ln)
		if errors.Is(err2, http.ErrServerClosed) {
			err2 = nil
		}
		errCh <- err2
	}()

	select {
	case <-ctx.Done():
		slog.InfoContext(ctx, "web", "msg", "Shutting down...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := s.Shutdown(shutdownCtx)
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
