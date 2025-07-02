// Copyright 2025 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Tool devpostdash is a devpost dashboard scraper.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/goccy/go-yaml"
)

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
	projects, err := scrapeDevpost(ctx, c)
	for _, p := range projects {
		// ID, URL, Image, Description, Team
		suffix := ""
		if p.Winner {
			suffix = " (winner)"
		}
		fmt.Printf("- %s %s%s\n", p.Title, p.Tagline, suffix)
		fmt.Printf("  %s\n", p.Team)
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
