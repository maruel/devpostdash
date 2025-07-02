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
	"sync"
	"time"
)

//go:embed templates/*.html
var templatesFS embed.FS

var templates = template.Must(template.ParseFS(templatesFS, "templates/*.html"))

type webserver struct {
	d     *devpostClient
	mu    sync.Mutex
	cache map[string][]Project
}

func (s *webserver) handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.Lookup("root.html").Execute(w, nil); err != nil {
		slog.ErrorContext(r.Context(), "web", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (s *webserver) handleSiteRedirect(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	if project == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/site/"+project+"/cards", http.StatusSeeOther)
}

func (s *webserver) handleSite(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	t := r.PathValue("type")
	if t != "cards" && t != "table" {
		http.Redirect(w, r, "/site/"+project+"/cards", http.StatusSeeOther)
		return
	}

	s.mu.Lock()
	projects, ok := s.cache[project]
	s.mu.Unlock()

	ctx := r.Context()
	if !ok {
		var err error
		projects, err = s.d.fetchProjects(ctx, project)
		if err != nil {
			var herr *httpError
			if errors.As(err, &herr) {
				w.WriteHeader(herr.StatusCode)
				w.Write(herr.Body)
				return
			}
			slog.ErrorContext(ctx, "web", "err", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		s.mu.Lock()
		s.cache[project] = projects
		s.mu.Unlock()
	}
	data := map[string]any{
		"Title":    project,
		"Projects": projects,
	}
	if err := templates.Lookup(t+".html").Execute(w, data); err != nil {
		slog.ErrorContext(ctx, "web", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func runWebserver(ctx context.Context, host string, d *devpostClient) error {
	w := &webserver{
		d:     d,
		cache: map[string][]Project{},
	}
	mux := http.ServeMux{}
	mux.HandleFunc("GET /{$}", w.handleRoot)
	mux.HandleFunc("GET /site/{project}", w.handleSiteRedirect)
	mux.HandleFunc("GET /site/{project}/{type}", w.handleSite)

	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", host)
	if err != nil {
		return err
	}
	slog.InfoContext(ctx, "web", "listening", ln.Addr())
	s := &http.Server{Handler: &mux, ReadHeaderTimeout: 2 * time.Second}
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
