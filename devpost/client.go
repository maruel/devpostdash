// Copyright 2025 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package devpost fetches the hackathon projects from devpost.com
package devpost

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path"
	"strconv"
	"sync"
	"time"

	"github.com/maruel/devpostdash/dom"
	"golang.org/x/net/html"
)

type Person struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	AvatarURL string `json:"avatar_url"`
}

type Project struct {
	ID        string   `json:"id"`
	ShortName string   `json:"short_name"`
	Title     string   `json:"title"`
	URL       string   `json:"url"`
	Tagline   string   `json:"tagline"`
	Image     string   `json:"image"`
	Winner    bool     `json:"winner"`
	Team      []Person `json:"team"`
	Likes     int      `json:"likes"`

	// These are loaded by fetchProject:
	Description   string   `json:"description"`
	DescriptionMD string   `json:"description_md"`
	Tags          []string `json:"tags"`

	LastRefresh time.Time `json:"last_refresh"`
}

func (p *Project) Hash() string {
	p2 := *p
	p2.LastRefresh = time.Time{}
	b, _ := json.Marshal(&p2)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:16])
}

type Event struct {
	ID            string     `json:"id"`
	Projects      []*Project `json:"projects"`
	LastRefresh   time.Time  `json:"last_refresh"`
	LastRequested time.Time  `json:"last_requested,omitzero"`
}

type Client interface {
	io.Closer
	FetchProjects(ctx context.Context, eventID string) ([]*Project, error)
	FetchProject(ctx context.Context, p *Project) error
}

type client struct {
	c      http.Client
	header http.Header
}

func New(ctx context.Context, h http.RoundTripper) (Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	jar.SetCookies(&url.URL{Scheme: "https", Host: "devpost.com"}, []*http.Cookie{
		{Name: "platform.notifications.newsletter.dismissed", Value: "dismissed"},
	})
	out := &client{
		header: http.Header{
			"Referer":    []string{"https://vibe-coding-hackathon.devpost.com/rules"},
			"User-Agent": []string{"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/137.0.0.0 Safari/537.36"},
		},
		c: http.Client{Transport: h, Jar: jar},
	}
	// Load cookies.
	_, err = out.get(ctx, "https://devpost.com")
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (d *client) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := d.c.Do(req)
	if err != nil {
		return nil, err
	}
	bod, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		err = &HTTPError{StatusCode: resp.StatusCode, Body: bod}
	}
	return bod, err
}

func (d *client) Close() error {
	return nil
}

func (d *client) FetchProjects(ctx context.Context, eventID string) ([]*Project, error) {
	var projects []*Project
	var err error
	start := time.Now()
	defer func() {
		slog.InfoContext(ctx, "devpost", "projects", len(projects), "dur", time.Since(start), "err", err)
	}()
	for i := 1; ; i++ {
		// url := "https://" + d.name + ".devpost.com/project-gallery"
		url := fmt.Sprintf("https://%s.devpost.com/submissions/search?page=%d&sort=alpha&terms=&utf8=%%E2%%9C%%93", eventID, i)
		var bod []byte
		if bod, err = d.get(ctx, url); err != nil {
			return projects, err
		}
		// A bit of a hack but good enough.
		if bytes.Contains(bod, []byte("There are no submissions which match your criteria.")) {
			break
		}
		if bytes.Contains(bod, []byte("The hackathon managers haven't published this gallery yet, but hang tight!")) {
			break
		}
		var p []*Project
		if p, err = parseProjects(bytes.NewReader(bod)); err != nil {
			return projects, err
		}
		if len(p) == 0 {
			break
		}
		projects = append(projects, p...)
	}
	return projects, nil
}

func (d *client) FetchProject(ctx context.Context, project *Project) error {
	start := time.Now()
	var err error
	defer func() {
		slog.InfoContext(ctx, "devpost", "project", project.ShortName, "dur", time.Since(start), "err", err)
	}()

	var bod []byte
	if bod, err = d.get(ctx, project.URL); err != nil {
		return err
	}
	var doc *html.Node
	if doc, err = html.Parse(bytes.NewReader(bod)); err != nil {
		return err
	}
	if d := dom.FirstChild(doc, dom.Tag("div"), dom.ID("app-details-left")); d != nil {
		project.Description = dom.NodeText(d)
		project.DescriptionMD = dom.NodeMarkdown(d)
	}
	project.Tags = nil
	if d := dom.FirstChild(doc, dom.Tag("div"), dom.ID("built-with")); d != nil {
		for c := range dom.YieldChildren(d, dom.Tag("span"), dom.Class("cp-tag")) {
			project.Tags = append(project.Tags, dom.NodeText(c))
		}
	}
	project.LastRefresh = time.Now()
	return nil
}

type cachedClient struct {
	d           Client
	freshness   time.Duration
	autoRefresh time.Duration
	stopRefresh time.Duration
	cacheFile   string

	mu     sync.Mutex
	events map[string]*Event

	ctx    context.Context
	cancel context.CancelFunc
}

func NewCached(parentCtx context.Context, d Client, freshness, autoRefresh time.Duration, cacheFilePath string) (Client, error) {
	if freshness <= autoRefresh {
		return nil, fmt.Errorf("freshness must be less than autoRefresh")
	}
	ctx, cancel := context.WithCancel(parentCtx)
	c := &cachedClient{
		d:           d,
		freshness:   freshness,
		autoRefresh: autoRefresh,
		stopRefresh: 4 * time.Hour,
		cacheFile:   cacheFilePath,
		events:      map[string]*Event{},
		ctx:         ctx,
		cancel:      cancel,
	}
	if err := c.loadCache(); err != nil {
		return nil, err
	}
	go c.autoRefreshLoop()
	return c, nil
}

func (c *cachedClient) loadCache() error {
	f, err := os.Open(c.cacheFile)
	defer func() {
		slog.InfoContext(c.ctx, "devpost", "msg", "loaded cache", "err", err, "path", c.cacheFile, "events", len(c.events))
	}()
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	data := serializedCache{}
	if err = json.NewDecoder(f).Decode(&data); err != nil {
		return err
	}
	c.mu.Lock()
	c.events = data.Events
	c.mu.Unlock()
	return nil
}

func (c *cachedClient) Close() error {
	c.cancel()
	return c.saveCache()
}

func (c *cachedClient) saveCache() error {
	f, err := os.Create(c.cacheFile)
	defer func() {
		slog.InfoContext(c.ctx, "devpost", "msg", "saved cache", "err", err, "events", len(c.events))
	}()
	if err != nil {
		return err
	}
	defer f.Close()
	e := json.NewEncoder(f)
	e.SetIndent("", "  ")

	c.mu.Lock()
	data := serializedCache{Version: 1, Events: c.events}
	err = e.Encode(&data)
	c.mu.Unlock()
	return err
}

func (c *cachedClient) autoRefreshLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			eventID := ""
			var p *Project
			c.mu.Lock()
			for id, event := range c.events {
				// Do not auto-refresh events that have not been requested in a while.
				if !event.LastRequested.IsZero() && time.Since(event.LastRequested) > c.stopRefresh {
					continue
				}
				if since := time.Since(event.LastRefresh); since > c.autoRefresh {
					eventID = id
					break
				}
				for _, project := range event.Projects {
					if since := time.Since(project.LastRefresh); since > c.autoRefresh {
						p = project
						break
					}
				}
				if p != nil {
					break
				}
			}
			c.mu.Unlock()

			if p != nil {
				slog.InfoContext(c.ctx, "devpost", "msg", "auto-refreshing project", "projectID", p.ID)
				if err := c.fetchProject(c.ctx, p); err != nil {
					slog.ErrorContext(c.ctx, "devpost", "msg", "failed to auto-refresh project", "projectID", p.ID, "err", err)
				}
			} else if eventID != "" {
				slog.InfoContext(c.ctx, "devpost", "msg", "auto-refreshing event", "eventID", eventID)
				if _, err := c.fetchProjects(c.ctx, eventID); err != nil {
					slog.ErrorContext(c.ctx, "devpost", "msg", "failed to auto-refresh event", "eventID", eventID, "err", err)
				}
			}
		}
	}
}

func (c *cachedClient) FetchProjects(ctx context.Context, eventID string) ([]*Project, error) {
	c.mu.Lock()
	e := c.events[eventID]
	if e != nil {
		e.LastRequested = time.Now()
	}
	c.mu.Unlock()
	if e != nil && time.Since(e.LastRefresh) < c.freshness {
		return e.Projects, nil
	}
	return c.fetchProjects(ctx, eventID)
}

func (c *cachedClient) fetchProjects(ctx context.Context, eventID string) ([]*Project, error) {
	projects, err := c.d.FetchProjects(ctx, eventID)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.events[eventID]
	if e != nil {
		// There's an existing list of projects. Merge the new list into it.
		// Create a map of old projects for efficient lookup.
		oldProjects := make(map[string]*Project, len(e.Projects))
		for _, p := range e.Projects {
			oldProjects[p.ID] = p
		}
		// Update the project details.
		for _, p := range projects {
			if old, ok := oldProjects[p.ID]; ok {
				// Copy over the fields that are not fetched by fetchProjects.
				p.Description = old.Description
				p.DescriptionMD = old.DescriptionMD
				p.Tags = old.Tags
				p.LastRefresh = old.LastRefresh
			}
		}
	} else {
		e = &Event{ID: eventID}
		c.events[eventID] = e
		// This is the first time this event is fetched, so we are in the context
		// of a user request.
		e.LastRequested = time.Now()
	}
	e.Projects = projects
	e.LastRefresh = time.Now()
	return projects, nil
}

func (c *cachedClient) FetchProject(ctx context.Context, project *Project) error {
	if time.Since(project.LastRefresh) < c.freshness {
		return nil
	}
	return c.fetchProject(ctx, project)
}

func (c *cachedClient) fetchProject(ctx context.Context, project *Project) error {
	err := c.d.FetchProject(ctx, project)
	if err == nil {
		project.LastRefresh = time.Now()
	}
	return err
}

//

type serializedCache struct {
	Version int               `json:"version"`
	Events  map[string]*Event `json:"events"`
}

func parseProjects(r io.Reader) ([]*Project, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, err
	}
	galleryNode := dom.FirstChild(doc, dom.Tag("div"), dom.ID("submission-gallery"))
	if galleryNode == nil {
		// No gallery found on this page, which is the end of pagination
		return nil, nil
	}
	var projects []*Project
	for c := range dom.YieldChildren(galleryNode, dom.Tag("div"), dom.Class("gallery-item")) {
		p := parseProjectNode(c)
		projects = append(projects, &p)
	}
	return projects, nil
}

func parseProjectNode(n *html.Node) Project {
	p := Project{}
	p.ID = dom.NodeAttr(n, "data-software-id")
	if linkNode := dom.FirstChild(n, dom.Tag("a"), dom.Class("block-wrapper-link")); linkNode != nil {
		p.URL = dom.NodeAttr(linkNode, "href")
		p.ShortName = path.Base(p.URL)
		if imgNode := dom.FirstChild(linkNode, dom.Tag("img"), dom.Class("software_thumbnail_image")); imgNode != nil {
			p.Image = dom.NodeAttr(imgNode, "src")
		}
	}
	if titleNode := dom.FirstChild(n, dom.Tag("h5")); titleNode != nil {
		p.Title = dom.NodeText(titleNode)
	}
	if taglineNode := dom.FirstChild(n, dom.Tag("p"), dom.Class("tagline")); taglineNode != nil {
		p.Tagline = dom.NodeText(taglineNode)
	}
	if winnerNode := dom.FirstChild(n, dom.Tag("aside"), dom.Class("entry-badge")); winnerNode != nil {
		p.Winner = true
	}
	for c := range dom.YieldChildren(n, dom.Tag("span"), dom.Class("user-profile-link")) {
		if imgNode := dom.FirstChild(c, dom.Tag("img")); imgNode != nil {
			p.Team = append(p.Team, Person{Name: dom.NodeAttr(imgNode, "alt"), AvatarURL: dom.NodeAttr(imgNode, "src"), URL: dom.NodeAttr(c, "data-url")})
		}
	}
	if likeNode := dom.FirstChild(n, dom.Tag("span"), dom.Class("count"), dom.Class("like-count")); likeNode != nil {
		t, err := strconv.Atoi(dom.NodeText(likeNode))
		if err != nil {
			slog.Error("failed to parse like count", "project", p.ID, "err", err)
		}
		p.Likes = t
	}
	// Description is not directly available on the nroject card.
	return p
}

type HTTPError struct {
	StatusCode int
	Body       []byte
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("status %d: %s", e.StatusCode, e.Body)
}
