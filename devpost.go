// Copyright 2025 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"path"
	"strconv"
	"time"

	"github.com/maruel/devpostdash/dom"
	"golang.org/x/net/html"
)

type devpostClient struct {
	c      http.Client
	header http.Header
}

func newDevpostClient(ctx context.Context, h http.RoundTripper) (devpostClient, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return devpostClient{}, err
	}
	jar.SetCookies(&url.URL{Scheme: "https", Host: "devpost.com"}, []*http.Cookie{
		{Name: "platform.notifications.newsletter.dismissed", Value: "dismissed"},
	})
	out := devpostClient{
		header: http.Header{
			"Referer":    []string{"https://vibe-coding-hackathon.devpost.com/rules"},
			"User-Agent": []string{"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/137.0.0.0 Safari/537.36"},
		},
		c: http.Client{Transport: h, Jar: jar},
	}
	// Load cookies.
	_, err = out.get(ctx, "https://devpost.com")
	return out, err
}

func (d *devpostClient) refreshDescriptions(ctx context.Context, projects []Project) error {
	for i := range projects {
		if err := d.fetchProject(ctx, &projects[i]); err != nil {
			return err
		}
	}
	return nil
}

type httpError struct {
	StatusCode int
	Body       []byte
}

func (e httpError) Error() string {
	return fmt.Sprintf("status code: %d", e.StatusCode)
}

func (d *devpostClient) get(ctx context.Context, url string) ([]byte, error) {
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
		err = &httpError{StatusCode: resp.StatusCode, Body: bod}
	}
	return bod, err
}

type Person struct {
	Name      string
	URL       string
	AvatarURL string
}

type Project struct {
	ID            string // int?
	ShortName     string
	Title         string
	URL           string
	Tagline       string
	Image         string
	Winner        bool
	Team          []Person
	Description   string
	DescriptionMD string
	Likes         int
	Tags          []string
	LastRefresh   time.Time
}

func (d *devpostClient) fetchProjects(ctx context.Context, eventID string) ([]Project, error) {
	var projects []Project
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
		var p []Project
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

func parseProjects(r io.Reader) ([]Project, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, err
	}
	galleryNode := dom.FirstChild(doc, dom.Tag("div"), dom.ID("submission-gallery"))
	if galleryNode == nil {
		// No gallery found on this page, which is the end of pagination
		return nil, nil
	}
	var projects []Project
	for c := range dom.YieldChildren(galleryNode, dom.Tag("div"), dom.Class("gallery-item")) {
		projects = append(projects, parseProjectNode(c))
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

func (d *devpostClient) fetchProject(ctx context.Context, project *Project) error {
	start := time.Now()
	var err error
	defer func() {
		slog.InfoContext(ctx, "devpost", "project", project.ShortName, "dur", time.Since(start), "err", err)
	}()

	if time.Since(project.LastRefresh) < 2*time.Minute {
		return nil
	}

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
	if d := dom.FirstChild(doc, dom.Tag("div"), dom.ID("built-with")); d != nil {
		for c := range dom.YieldChildren(d, dom.Tag("span"), dom.Class("cp-tag")) {
			project.Tags = append(project.Tags, dom.NodeText(c))
		}
	}
	project.LastRefresh = time.Now()
	return nil
}
