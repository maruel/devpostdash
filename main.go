// Copyright 2025 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Tool devpostdash is a devpost dashboard scraper.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/maruel/roundtrippers"
	"golang.org/x/sync/errgroup"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"
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

// httpRecorder records HTTP requests and responses to testdata/.
func httpRecorder(ctx context.Context) (*recorder.Recorder, error) {
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
	h := &roundtrippers.Capture{Transport: http.DefaultTransport, C: ch}
	rr, err := recorder.New("testdata/main",
		recorder.WithMode(recorder.ModeRecordOnce),
		recorder.WithSkipRequestLatency(true),
		recorder.WithRealTransport(h),
		recorder.WithHook(trimResponseHeaders, recorder.AfterCaptureHook),
	)
	if err != nil {
		return nil, err
	}
	return rr, nil
}

func trimResponseHeaders(i *cassette.Interaction) error {
	i.Request.Headers.Del("Authorization")
	i.Request.Headers.Del("X-Request-Id")
	i.Response.Headers.Del("Set-Cookie")
	i.Response.Headers.Del("Date")
	i.Response.Headers.Del("X-Request-Id")
	i.Response.Duration = i.Response.Duration.Round(time.Millisecond)
	return nil
}

func mainImpl() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	verbose := flag.Bool("verbose", false, "verbose mode")
	record := flag.Bool("record", false, "record mode")
	host := flag.String("host", ":8080", "host")
	dump := flag.Bool("dump", false, "dump mode")
	flag.Parse()

	if flag.NFlag() != 0 {
		return errors.New("unknown flags")
	}
	if *verbose {
		// log.SetLevel(log.DebugLevel)
	}
	config, err := os.ReadFile("config.yml")
	if err != nil {
		return err
	}
	var c Config
	if err := yaml.Unmarshal(config, &c); err != nil {
		return err
	}

	h := http.DefaultTransport
	if *record {
		rr, err := httpRecorder(ctx)
		if err != nil {
			return err
		}
		defer rr.Stop()
		h = rr
	}
	d, err := newDevpostClient(&c, &roundtrippers.Throttle{Transport: h, QPS: 1})
	if err != nil {
		return err
	}

	projects, err := d.fetchProjects(ctx)
	if err != nil {
		return err
	}
	if len(projects) == 0 {
		return errors.New("no projects found")
	}
	if *dump {
		printProjects(projects)
		return nil
	}
	log.Printf("Found %d projects", len(projects))
	return runWebserver(ctx, *host, projects)
}

func main() {
	if err := mainImpl(); err != nil {
		if err != context.Canceled {
			fmt.Fprintln(os.Stderr, "devpostdash:", err)
			os.Exit(1)
		}
	}
}
