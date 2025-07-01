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
	"iter"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/signal"
	"slices"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/maruel/roundtrippers"
	"golang.org/x/net/html"
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
	ID             string
	Title          string
	URL            string
	Tagline        string
	Image          string
	Description    string
	Winner         bool
	Team           string
	SubmissionDate string
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
	galleryNode := getFirstChild(doc, withTag("div"), withID("submission-gallery"))
	if galleryNode == nil {
		// No gallery found on this page, which is the end of pagination
		return nil, nil
	}
	var projects []project
	for c := range yieldChildren(galleryNode, withTag("div"), withClass("gallery-item")) {
		projects = append(projects, parseProjectNode(c))
	}
	return projects, nil
}

func parseProjectNode(n *html.Node) project {
	p := project{}
	if idNode := getFirstChild(n, withTag("a"), withClass("block-link")); idNode != nil {
		p.ID = getNodeAttr(idNode, "data-content-id")
		p.URL = getNodeAttr(idNode, "href")
	}
	if titleNode := getFirstChild(n, withTag("h5"), withClass("content-title")); titleNode != nil {
		if aNode := getFirstChild(titleNode, withTag("a")); aNode != nil {
			p.Title = aNode.FirstChild.Data
		}
	}
	if taglineNode := getFirstChild(n, withTag("p"), withClass("tagline")); taglineNode != nil {
		p.Tagline = strings.TrimSpace(taglineNode.FirstChild.Data)
	}
	if imgNode := getFirstChild(n, withTag("img"), withClass("project-card-img")); imgNode != nil {
		p.Image = getNodeAttr(imgNode, "src")
	}
	if descNode := getFirstChild(n, withTag("div"), withClass("description")); descNode != nil {
		p.Description = descNode.FirstChild.Data
	}
	if winnerNode := getFirstChild(n, withTag("div"), withClass("winner-flag")); winnerNode != nil {
		p.Winner = true
	}
	if teamNode := getFirstChild(n, withTag("p"), withClass("team-name")); teamNode != nil {
		if aNode := getFirstChild(teamNode, withTag("a")); aNode != nil {
			p.Team = aNode.FirstChild.Data
		} else {
			p.Team = teamNode.FirstChild.Data
		}
	}
	if dateNode := getFirstChild(n, withTag("p"), withClass("submission-date")); dateNode != nil {
		p.SubmissionDate = dateNode.FirstChild.Data
	}
	return p
}

func (d *devpostClient) fetchProject(ctx context.Context, projectID string) ([]byte, error) {
	url := "https://" + d.name + ".devpost.com/submissions/" + projectID
	bod, err := d.get(ctx, url)
	// TODO: Parse the project page and extract useful information.
	return bod, err
}

//

// trimResponseHeaders trims API key and noise from the recording.
func trimResponseHeaders(i *cassette.Interaction) error {
	i.Request.Headers.Del("Authorization")
	i.Request.Headers.Del("X-Request-Id")
	i.Response.Headers.Del("Set-Cookie")
	i.Response.Headers.Del("Date")
	i.Response.Headers.Del("X-Request-Id")
	i.Response.Duration = i.Response.Duration.Round(time.Millisecond)
	return nil
}

// Generic HTML parsing code.

type nodeSelector func(*html.Node) bool

func withTag(tagName string) nodeSelector {
	return func(n *html.Node) bool {
		return n.Type == html.ElementNode && n.Data == tagName
	}
}

func withAttr(key, val string) nodeSelector {
	return func(n *html.Node) bool {
		return n.Type == html.ElementNode && getNodeAttr(n, key) == val
	}
}

func withClass(className string) nodeSelector {
	return func(n *html.Node) bool {
		return n.Type == html.ElementNode && slices.Contains(strings.Split(getNodeAttr(n, "class"), " "), className)
	}
}

func withID(id string) nodeSelector {
	return withAttr("id", id)
}

// yieldChildren travel the tree with the filter specified, traversing depth first.
func yieldChildren(n *html.Node, filters ...nodeSelector) iter.Seq[*html.Node] {
	return func(yield func(*html.Node) bool) {
		ok := true
		for _, f := range filters {
			if !f(n) {
				ok = false
				break
			}
		}
		if ok && !yield(n) {
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			for m := range yieldChildren(c, filters...) {
				if !yield(m) {
					return
				}
			}
		}
	}
}

// getFirstChild travel the tree with the filter specified and returns the first node found.
func getFirstChild(n *html.Node, filters ...nodeSelector) *html.Node {
	for n := range yieldChildren(n, filters...) {
		return n
	}
	return nil
}

// getNodeAttr returns the attribute of an html node if it exists.
func getNodeAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
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

	h := &roundtrippers.Throttle{Transport: http.DefaultTransport, QPS: 1}
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
