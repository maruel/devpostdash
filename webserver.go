// Copyright 2025 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/maruel/devpostdash/devpost"
)

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed all:static
var staticFS embed.FS

var templates = template.Must(template.New("").Funcs(template.FuncMap{"jsonMarshal": jsonMarshal}).ParseFS(templatesFS, "templates/*.html"))

func jsonMarshal(v any) (template.JS, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return template.JS(b), nil
}

type webserver struct {
	d devpost.Client
	r *roaster
}

func (s *webserver) handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	ctx := r.Context()
	if err := templates.Lookup("page_root.html").Execute(w, nil); err != nil {
		handleError(ctx, w, err)
	}
}

func (s *webserver) handleAbout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	ctx := r.Context()
	if err := templates.Lookup("page_about.html").Execute(w, nil); err != nil {
		handleError(ctx, w, err)
	}
}

func (s *webserver) handleEventRedirect(w http.ResponseWriter, r *http.Request) {
	eventID := r.PathValue("eventID")
	if eventID == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/event/"+eventID+"/card", http.StatusSeeOther)
}

func handleError(ctx context.Context, w http.ResponseWriter, err error) {
	slog.ErrorContext(ctx, "web", "err", err)
	var herr *devpost.HTTPError
	if errors.As(err, &herr) {
		w.WriteHeader(herr.StatusCode)
		_, _ = w.Write(herr.Body)
		return
	}
	http.Error(w, "Internal Server Error", http.StatusInternalServerError)
}

func (s *webserver) handleEvent(w http.ResponseWriter, r *http.Request) {
	eventID := r.PathValue("eventID")
	pageType := r.PathValue("type")
	// Load the corresponding page_TYPE.html page under templates/.
	tmpl := templates.Lookup("page_" + pageType + ".html")
	if tmpl == nil {
		http.Redirect(w, r, "/event/"+eventID+"/card", http.StatusSeeOther)
		return
	}

	ctx := r.Context()
	out, err := s.getProjects(ctx, eventID)
	if err != nil {
		handleError(ctx, w, err)
		return
	}
	data := map[string]any{
		"Title":    eventID,
		"EventID":  eventID,
		"Projects": out,
	}
	if err := tmpl.Execute(w, data); err != nil {
		handleError(ctx, w, err)
	}
}

func (s *webserver) apiEvent(w http.ResponseWriter, r *http.Request) {
	eventID := r.PathValue("eventID")
	ctx := r.Context()
	out, err := s.getProjects(ctx, eventID)
	if err != nil {
		handleError(ctx, w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		handleError(ctx, w, err)
	}
}

func (s *webserver) apiRoast(w http.ResponseWriter, r *http.Request) {
	var roastReq struct {
		EventID   string `json:"event_id"`
		ProjectID string `json:"project_id"`
	}
	ctx := r.Context()
	if err := json.NewDecoder(r.Body).Decode(&roastReq); err != nil {
		handleError(ctx, w, &devpost.HTTPError{StatusCode: http.StatusBadRequest, Body: []byte(err.Error())})
		return
	}
	p, err := s.getProject(ctx, roastReq.EventID, roastReq.ProjectID)
	if err != nil {
		handleError(ctx, w, err)
		return
	}
	roast, err := s.r.doRoast(ctx, p)
	if err != nil {
		handleError(ctx, w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"content": roast}); err != nil {
		handleError(ctx, w, err)
	}
}

func (s *webserver) getProjects(ctx context.Context, eventID string) ([]*devpost.Project, error) {
	if eventID == "mock" {
		return devpostProjects, nil
	}
	projects, err := s.d.FetchProjects(ctx, eventID)
	if err != nil {
		return nil, err
	}
	sort.Slice(projects, func(i, j int) bool {
		return projects[i].Likes > projects[j].Likes
	})
	out := make([]*devpost.Project, 0, len(projects))
	for _, p := range projects {
		p2 := *p
		p2.LastRefresh = time.Time{}
		out = append(out, &p2)
	}
	if len(out) == 0 {
		out = devpostProjects
	}
	return out, nil
}

var devpostProjects = []*devpost.Project{
	{
		ID:        "-1",
		ShortName: "devpostdash",
		Title:     "Devpost Dashboard",
		Tagline:   "Awesome dashboard for our hackathon",
		URL:       "https://github.com/maruel/devpostdash",
		Winner:    true,
		Team: []devpost.Person{
			{
				Name:      "Marc-Antoine Ruel",
				URL:       "https://devpost.com/maruel",
				AvatarURL: "https://lh3.googleusercontent.com/a/ACg8ocLSOzFuWl-UhprsbeOvk-eYdoA-HngsePYGLguoUpKxO9dI-XLmzA=s96-c",
			},
		},
		Likes:       31337,
		Tags:        []string{"devpost", "dashboard", "roast"},
		Description: "This project fetches the data from devpost.com using webscraping. This is because devpost.com has no API. This is a bit frustrating. The server presents a nice interactive web UI that can be used during competitions.",
		Image:       "/static/img/dancing-gopher.gif",
	},
	{
		ID:        "-2",
		ShortName: "soon",
		Title:     "You project here!",
		Tagline:   "Awesome project created during the hackathon",
		URL:       "https://example.com",
		Team: []devpost.Person{
			{
				Name: "You",
				URL:  "https://example.com",
			},
		},
		Likes:       1,
		Tags:        []string{"soon"},
		Description: "Solve real world problems, or not, and win prizes!",
	},
}

func (s *webserver) getProject(ctx context.Context, eventID, projectID string) (*devpost.Project, error) {
	if projectID == "-1" {
		return devpostProjects[0], nil
	}
	if projectID == "-2" {
		return devpostProjects[1], nil
	}
	projects, err := s.d.FetchProjects(ctx, eventID)
	if err != nil {
		return nil, err
	}
	var p *devpost.Project
	for i := range projects {
		if projects[i].ID == projectID {
			p = projects[i]
			break
		}
	}
	if p == nil {
		return nil, &devpost.HTTPError{
			StatusCode: http.StatusNotFound,
			Body:       []byte(fmt.Sprintf("project %q not found", eventID+"/"+projectID)),
		}
	}
	// Refresh description and tags for the single project
	if err := s.d.FetchProject(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		defer func() {
			slog.InfoContext(r.Context(), "web", "path", r.URL.Path, "ip", getRealIP(r), "dur", time.Since(start))
		}()
		next.ServeHTTP(w, r)
	})
}

// getRealIP extracts the client's real IP address from an HTTP request,
// taking into account X-Forwarded-For or other proxy headers.
func getRealIP(r *http.Request) net.IP {
	// Check X-Forwarded-For header (most common proxy header)
	if xForwardedFor := r.Header.Get("X-Forwarded-For"); xForwardedFor != "" { // X-Forwarded-For can contain multiple IPs, the client's IP is the first one
		ip := net.ParseIP(strings.TrimSpace(strings.Split(xForwardedFor, ",")[0]))
		if ip != nil {
			return ip
		}
	}

	// Check X-Real-IP header (used by some proxies)
	if xRealIP := r.Header.Get("X-Real-IP"); xRealIP != "" {
		if ip := net.ParseIP(xRealIP); ip != nil {
			return ip
		}
	}

	// If no proxy headers found, get the remote address
	if remoteAddr := r.RemoteAddr; remoteAddr != "" {
		// RemoteAddr might be in the format IP:port
		if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
			if ip := net.ParseIP(host); ip != nil {
				return ip
			}
		} else {
			// If SplitHostPort fails, try parsing the whole RemoteAddr as an IP
			if ip := net.ParseIP(remoteAddr); ip != nil {
				return ip
			}
		}
	}
	return nil
}

func newWebServerHandler(d devpost.Client, r *roaster) http.Handler {
	w := &webserver{d: d, r: r}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", w.handleRoot)
	mux.HandleFunc("GET /about", w.handleAbout)
	mux.HandleFunc("GET /event/{eventID}", w.handleEventRedirect)
	mux.HandleFunc("GET /event/{eventID}/{type}", w.handleEvent)
	mux.HandleFunc("GET /api/events/{eventID}", w.apiEvent)
	mux.HandleFunc("POST /api/roast", w.apiRoast)
	staticContent, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticContent))))
	return loggingMiddleware(mux)
}

func runWebserver(ctx context.Context, host string, d devpost.Client, r *roaster) error {
	handler := newWebServerHandler(d, r)
	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", host)
	if err != nil {
		return err
	}
	slog.InfoContext(ctx, "web", "listening", ln.Addr())
	s := &http.Server{Handler: handler, ReadHeaderTimeout: 2 * time.Second}
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
			return err
		}
		return <-errCh
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("server error: %w", err)
		}
	}
	return nil
}
