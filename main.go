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
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/lmittmann/tint"
	"github.com/maruel/roundtrippers"
	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
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

func watchExecutable(ctx context.Context, cancel context.CancelFunc) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create file watcher: %w", err)
	}
	go func() {
		defer w.Close()
		for {
			select {
			case event, ok := <-w.Events:
				if !ok {
					return
				}
				// Detect writes or chmod events which may indicate a modification
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Chmod) {
					slog.InfoContext(ctx, "citygpt", "msg", "Executable file was modified, initiating shutdown...")
					cancel()
					return
				}
			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				slog.WarnContext(ctx, "citygpt", "msg", "Error watching executable", "err", err)
			case <-ctx.Done():
				return
			}
		}
	}()
	if err := w.Add(exePath); err != nil {
		return fmt.Errorf("failed to watch executable: %w", err)
	}
	return nil
}

func mainImpl() error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	defer cancel()
	Level := &slog.LevelVar{}
	Level.Set(slog.LevelInfo)
	logger := slog.New(tint.NewHandler(colorable.NewColorable(os.Stderr), &tint.Options{
		Level:      Level,
		TimeFormat: "15:04:05.000", // Like time.TimeOnly plus milliseconds.
		NoColor:    !isatty.IsTerminal(os.Stderr.Fd()),
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			val := a.Value.Any()
			skip := false
			switch t := val.(type) {
			case string:
				skip = t == ""
			case bool:
				skip = !t
			case uint64:
				skip = t == 0
			case int64:
				skip = t == 0
			case float64:
				skip = t == 0
			case time.Time:
				skip = t.IsZero()
			case time.Duration:
				skip = t == 0
			case nil:
				skip = true
			}
			if skip {
				return slog.Attr{}
			}
			return a
		},
	}))
	slog.SetDefault(logger)
	if err := watchExecutable(ctx, cancel); err != nil {
		return err
	}

	verbose := flag.Bool("verbose", false, "verbose mode")
	record := flag.Bool("record", false, "record mode")
	host := flag.String("host", ":8080", "host")
	dump := flag.String("dump", "", "dump mode")
	flag.Parse()

	if flag.NArg() != 0 {
		return errors.New("unknown arguments")
	}
	if *verbose {
		Level.Set(slog.LevelDebug)
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
	d, err := newDevpostClient(ctx, &roundtrippers.Throttle{Transport: h, QPS: 1})
	if err != nil {
		return err
	}

	if *dump != "" {
		projects, err := d.fetchProjects(ctx, *dump)
		if err != nil {
			return err
		}
		if len(projects) == 0 {
			return errors.New("no projects found")
		}
		printProjects(projects)
		return nil
	}
	return runWebserver(ctx, *host, &d)
}

func main() {
	if err := mainImpl(); err != nil {
		if err != context.Canceled {
			fmt.Fprintln(os.Stderr, "devpostdash:", err)
			os.Exit(1)
		}
	}
}
