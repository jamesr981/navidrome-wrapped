package main

import (
	_ "embed"
	"encoding/json"
	"strings"
)

//go:embed story.html
var storyTemplate string

//go:embed index.html
var indexTemplate string

const dataPlaceholder = "/*__WRAPPED_DATA__*/null"

// inject places JSON data into a template, escaped so it cannot break out of the <script> tag.
func inject(tpl string, data any) ([]byte, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	js := string(b)
	js = strings.ReplaceAll(js, "</", `<\/`)
	js = strings.ReplaceAll(js, "<!--", `<\!--`)
	js = strings.ReplaceAll(js, " ", ` `)
	js = strings.ReplaceAll(js, " ", ` `)
	return []byte(strings.Replace(tpl, dataPlaceholder, js, 1)), nil
}

func renderStory(st *Stats) ([]byte, error) { return inject(storyTemplate, st) }

func renderIndex(year int, entries []indexEntry) ([]byte, error) {
	type u struct {
		Username    string `json:"username"`
		DisplayName string `json:"displayName"`
		File        string `json:"file"`
		Plays       int    `json:"plays"`
		Minutes     int    `json:"minutes"`
		TopArtist   string `json:"topArtist"`
		Personality string `json:"personality"`
	}
	list := make([]u, 0, len(entries))
	for _, e := range entries {
		list = append(list, u(e))
	}
	return inject(indexTemplate, map[string]any{"year": year, "users": list})
}
