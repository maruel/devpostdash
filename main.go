// Copyright 2025 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Tool devpostdash is a devpost dashboard scraper.
package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/maruel/devpostdash/dom"
	"github.com/maruel/roundtrippers"
	"golang.org/x/net/html"
	"golang.org/x/sync/errgroup"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"
)

// Config is the content of config.yml.
type Config struct {
	Name   string
	ID     string
	Cookie string
}

type devpostClient struct {
	name   string
	id     string
	c      http.Client
	header http.Header
}

func newDevpostClient(c *Config, h http.RoundTripper) (devpostClient, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		panic(err)
	}
	out := devpostClient{
		name: c.Name,
		id:   c.ID,
		header: http.Header{
			"Cookie":     []string{c.Cookie},
			"Referer":    []string{"https://vibe-coding-hackathon.devpost.com/rules"},
			"User-Agent": []string{"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/137.0.0.0 Safari/537.36"},
		},
		c: http.Client{Transport: h, Jar: jar},
	}
	return out, nil
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
		err = fmt.Errorf("status code: %d", resp.StatusCode)
	}
	return bod, err
}

/*
func (d *devpost) login(ctx context.Context) ([]byte, error) {
	url := "https://devpost.com/users/login"
	bod, err := d.get(ctx, url)
	return bod, err
}
*/

/*
func (d *devpost) fetchProjectsInternal(ctx context.Context) ([]byte, error) {
	url := "https://manage.devpost.com/challenges/" + d.id + "/dashboard/submissions"
	bod, err := d.get(ctx, url)
	return bod, err
}
*/

type project struct {
	ID          string
	Title       string
	URL         string
	Tagline     string
	Image       string
	Winner      bool
	Team        string
	Description string
}

func (d *devpostClient) fetchProjects(ctx context.Context) ([]project, error) {
	var projects []project
	for i := 1; ; i++ {
		// url := "https://" + d.name + ".devpost.com/project-gallery"
		url := fmt.Sprintf("https://%s.devpost.com/submissions/search?page=%d&sort=alpha&terms=&utf8=%%E2%%9C%%93", d.name, i)
		bod, err := d.get(ctx, url)
		if err != nil {
			return projects, err
		}
		// A bit of a hack but good enough.
		if bytes.Contains(bod, []byte("There are no submissions which match your criteria.")) {
			break
		}
		p, err := parseProjects(bytes.NewReader(bod))
		if err != nil {
			return projects, err
		}
		projects = append(projects, p...)
	}
	return projects, nil
}

func parseProjects(r io.Reader) ([]project, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, err
	}
	galleryNode := dom.FirstChild(doc, dom.Tag("div"), dom.ID("submission-gallery"))
	if galleryNode == nil {
		// No gallery found on this page, which is the end of pagination
		return nil, nil
	}
	var projects []project
	for c := range dom.YieldChildren(galleryNode, dom.Tag("div"), dom.Class("gallery-item")) {
		projects = append(projects, parseProjectNode(c))
	}
	return projects, nil
}

func parseProjectNode(n *html.Node) project {
	p := project{}
	p.ID = dom.NodeAttr(n, "data-software-id")
	if linkNode := dom.FirstChild(n, dom.Tag("a"), dom.Class("block-wrapper-link")); linkNode != nil {
		p.URL = dom.NodeAttr(linkNode, "href")
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
	var teamNames []string
	for c := range dom.YieldChildren(n, dom.Tag("span"), dom.Class("user-profile-link")) {
		if imgNode := dom.FirstChild(c, dom.Tag("img")); imgNode != nil {
			teamNames = append(teamNames, dom.NodeAttr(imgNode, "alt"))
		}
	}
	p.Team = strings.Join(teamNames, ", ")
	// Description is not directly available on the project card.
	return p
}

func (d *devpostClient) fetchProject(ctx context.Context, project *project) error {
	// url := "https://" + d.name + ".devpost.com/submissions/" + project.ID
	bod, err := d.get(ctx, project.URL)
	if err != nil {
		return err
	}
	doc, err := html.Parse(bytes.NewReader(bod))
	if err != nil {
		return err
	}
	if d := dom.FirstChild(doc, dom.Tag("div"), dom.ID("app-details-left")); d != nil {
		project.Description = dom.NodeMarkdown(d)
	}
	return nil
}

//

func trimResponseHeaders(i *cassette.Interaction) error {
	i.Request.Headers.Del("Authorization")
	i.Request.Headers.Del("X-Request-Id")
	i.Response.Headers.Del("Set-Cookie")
	i.Response.Headers.Del("Date")
	i.Response.Headers.Del("X-Request-Id")
	i.Response.Duration = i.Response.Duration.Round(time.Millisecond)
	return nil
}

//

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

	ch := make(chan roundtrippers.Record)
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		for i := 0; ; i++ {
			select {
			case r, ok := <-ch:
				if !ok {
					return nil
				}
				if r.Response != nil {
					f, err := os.Create(fmt.Sprintf("testdata/get%03d.html", i))
					if err != nil {
						return err
					}
					_, err = io.Copy(f, r.Response.Body)
					_ = f.Close()
					if err != nil {
						return err
					}
				}
			case <-ctx.Done():
				return nil
			}
		}
	})
	h := &roundtrippers.Capture{
		Transport: &roundtrippers.Throttle{
			Transport: http.DefaultTransport, QPS: 1,
		},
		C: ch,
	}
	rr, err := recorder.New("testdata/main",
		recorder.WithMode(recorder.ModeRecordOnce),
		recorder.WithSkipRequestLatency(true),
		recorder.WithRealTransport(h),
		recorder.WithHook(trimResponseHeaders, recorder.AfterCaptureHook),
	)
	if err != nil {
		return err
	}
	defer rr.Stop()
	d, err := newDevpostClient(&c, rr)
	if err != nil {
		return err
	}
	projects, err := d.fetchProjects(ctx)
	if err != nil {
		return err
	}
	for i := range projects {
		if err = d.fetchProject(ctx, &projects[i]); err != nil {
			return err
		}
	}
	for _, p := range projects {
		fmt.Printf("- %#v\n", p)
	}
	return err
}

func main() {
	if err := mainImpl(); err != nil {
		if err != context.Canceled {
			fmt.Fprintln(os.Stderr, "devpostdash:", err)
			os.Exit(1)
		}
	}
}
