// Copyright 2025 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Tool devpostdash is a devpost dashboard scraper.
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"

	"github.com/goccy/go-yaml"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"
)

type Config struct {
	Name   string
	ID     string
	Cookie string
}

type devpost struct {
	name   string
	id     string
	header http.Header
	h      http.RoundTripper
}

func newDevpost(c *Config, h http.RoundTripper) (devpost, error) {
	out := devpost{name: c.Name, id: c.ID, h: h}
	out.header = http.Header{
		"Cookie":     []string{c.Cookie},
		"Referer":    []string{"https://vibe-coding-hackathon.devpost.com/rules"},
		"User-Agent": []string{"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/137.0.0.0 Safari/537.36"},
	}
	return out, nil
}

func (d *devpost) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	c := http.Client{Transport: d.h}
	resp, err := c.Do(req)
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

func (d *devpost) fetchProjects(ctx context.Context) ([]byte, error) {
	// url := "https://" + d.name + ".devpost.com/project-gallery"
	url := "https://manage.devpost.com/challenges/" + d.id + "/dashboard/submissions"
	bod, err := d.get(ctx, url)
	return bod, err
}

func (d *devpost) fetchProject(ctx context.Context, project string) ([]byte, error) {
	url := "https://" + d.name + ".devpost.com/submissions/" + project
	bod, err := d.get(ctx, url)
	return bod, err
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

	h := http.DefaultTransport
	rr, err := recorder.New("testdata/main",
		recorder.WithMode(recorder.ModeRecordOnce),
		recorder.WithSkipRequestLatency(true),
		recorder.WithRealTransport(h),
	)
	if err != nil {
		return err
	}
	defer rr.Stop()
	d, err := newDevpost(&c, rr)
	if err != nil {
		return err
	}
	bod, err := d.fetchProjects(ctx)
	if err != nil {
		return err
	}
	fmt.Println(string(bod))
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
