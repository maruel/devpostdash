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
	"os/user"
	"path/filepath"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/lmittmann/tint"
	"github.com/maruel/devpostdash/devpost"
	"github.com/maruel/genai"
	"github.com/maruel/genai/base"
	"github.com/maruel/genai/providers"
	"github.com/maruel/roundtrippers"
	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
	"golang.org/x/sync/errgroup"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"
)

func printProjects(projects []*devpost.Project) {
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
					slog.InfoContext(ctx, "devpostdash", "msg", "Executable file was modified, initiating shutdown...")
					cancel()
					return
				}
			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				slog.WarnContext(ctx, "devpostdash", "msg", "Error watching executable", "err", err)
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
				if t == 0 {
					skip = true
				} else {
					if t < 10*time.Millisecond {
						t = t.Round(time.Microsecond)
					} else if t < 10*time.Second {
						t = t.Round(time.Millisecond)
					} else if t < 10*time.Minute {
						t = t.Round(time.Second)
					} else if t < 10*time.Hour {
						t = t.Round(time.Minute)
					}
					a.Value = slog.DurationValue(t)
				}
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
	provider := flag.String("provider", "cerebras", "LLM provider to use")
	model := flag.String("model", base.PreferredGood, "LLM model to use")
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
	u, err := user.Current()
	if err != nil {
		return err
	}
	cacheDir := filepath.Join(u.HomeDir, ".cache", "devpostdash")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return err
	}
	rawDevpostClient, err := devpost.New(ctx, &roundtrippers.Throttle{Transport: h, QPS: 1})
	if err != nil {
		return err
	}
	defer rawDevpostClient.Close()
	// Refresh every 5 minutes, and cache for 1 hour.
	d, err := devpost.NewCached(ctx, rawDevpostClient, 1*time.Hour, 5*time.Minute, filepath.Join(cacheDir, "devpost.json"))
	if err != nil {
		return err
	}
	defer d.Close()

	if *dump != "" {
		projects, err := d.FetchProjects(ctx, *dump)
		if err != nil {
			return err
		}
		if len(projects) == 0 {
			return errors.New("no projects found")
		}
		printProjects(projects)
		return nil
	}

	var c genai.ProviderGen
	if *provider != "" {
		prov := providers.All[*provider]
		if prov == nil {
			return fmt.Errorf("unknown provider %q", *provider)
		}
		f := func(h http.RoundTripper) http.RoundTripper {
			return &roundtrippers.Throttle{Transport: h, QPS: 0.5}
		}
		cl, err := prov(*model, f)
		if err != nil {
			return err
		}
		ok := false
		if c, ok = cl.(genai.ProviderGen); !ok {
			return fmt.Errorf("%T does not implement genai.ProviderGen", *provider)
		}
	}
	r, err := newRoaster(c, filepath.Join(cacheDir, "roaster.json"))
	if err != nil {
		return err
	}
	defer r.Close()
	return runWebserver(ctx, *host, d, r)
}

func main() {
	if err := mainImpl(); err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			fmt.Fprintln(os.Stderr, "devpostdash:", err)
			os.Exit(1)
		}
	}
}
