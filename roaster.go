// Copyright 2025 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/maruel/genai"
)

type Roast struct {
	Content     string    `json:"content"`
	LastRefresh time.Time `json:"last_refresh"`
	Hash        string    `json:"hash"`
}

type roaster struct {
	llm       genai.ProviderGen
	cacheFile string

	mu     sync.Mutex
	roasts map[string]*Roast
}

func newRoaster(c genai.ProviderGen, cacheFile string) (*roaster, error) {
	r := &roaster{llm: c, cacheFile: cacheFile, roasts: map[string]*Roast{}}
	err := r.loadCache()
	return r, err
}

func (r *roaster) loadCache() error {
	f, err := os.Open(r.cacheFile)
	defer slog.Info("web", "msg", "loaded cache", "err", err, "path", r.cacheFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	data := serializedRoaster{}
	if err = json.NewDecoder(f).Decode(&data); err != nil {
		return err
	}
	r.mu.Lock()
	r.roasts = data.Roasts
	r.mu.Unlock()
	return nil
}

func (r *roaster) Close() error {
	f, err := os.Create(r.cacheFile)
	defer slog.Info("web", "msg", "saved cache", "err", err)
	if err != nil {
		return err
	}
	defer f.Close()
	data := serializedRoaster{Version: 1, Roasts: r.roasts}
	err = json.NewEncoder(f).Encode(&data)
	return err
}

const (
	hardRoast = "Roast the following project. Be funny and concise. Reply with only one hard hitting sentence, nothing else."
	softRoast = "Roast the following project. Be funny and concise. Reply with only one lighthearted sentence, nothing else."
)

func (r *roaster) doRoast(ctx context.Context, p *Project) (string, error) {
	r.mu.Lock()
	roast := r.roasts[p.ID]
	r.mu.Unlock()

	// Roast again whenever the project properties have changed.
	if ph := p.Hash(); roast == nil || ph != roast.Hash {
		teamNames := make([]string, len(p.Team))
		for i, p := range p.Team {
			teamNames[i] = p.Name
		}
		roastType := softRoast
		if slices.Contains(p.Tags, "roast") {
			roastType = hardRoast
		}
		winner := ""
		if p.Winner {
			winner = "The team is a winner from the competition.\n"
		}
		prompt := fmt.Sprintf(
			"%s\nProject name: %s\nTag line: %s\nTeam members: %s\n%sTags: %s\nWhole Description:\n%s",
			roastType,
			p.Title,
			p.Tagline,
			strings.Join(teamNames, ", "),
			winner,
			strings.Join(p.Tags, ", "),
			p.Description)
		msgs := genai.Messages{genai.NewTextMessage(genai.User, prompt)}
		resp, err := r.llm.GenSync(ctx, msgs, &genai.OptionsText{Temperature: 1.0})
		if err != nil {
			return "", err
		}
		roast = &Roast{Content: resp.AsText(), LastRefresh: time.Now()}
		if roast.Content == "" {
			return "", errors.New("no content generated")
		}
		roast.Hash = ph
		slog.InfoContext(ctx, "roast", "content", roast)
		r.mu.Lock()
		r.roasts[p.ID] = roast
		r.mu.Unlock()
	}
	return roast.Content, nil
}

//

type serializedRoaster struct {
	Version int               `json:"version"`
	Roasts  map[string]*Roast `json:"roasts"`
}
