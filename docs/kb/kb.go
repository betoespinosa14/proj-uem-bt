// Package kb holds BEACON's knowledge base: one Markdown file per article,
// embedded into the server and shown to owners and admins in the console.
//
// Each file starts with a header between "---" lines:
//
//	---
//	title: Backups and restore
//	section: Operations
//	order: 40
//	summary: Nightly database dumps, where they live, how to restore.
//	reviewed: 2026-11-20
//	---
//
// The file name (without .md) is the article's address, and other articles
// link to it as [text](name.md).
package kb

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
)

//go:embed *.md
var files embed.FS

// Sections in the order the console lists them.
var Sections = []string{"Start here", "Architecture", "Operations", "Troubleshooting", "Security", "FAQ"}

// Article is one knowledge base page.
type Article struct {
	Slug     string `json:"slug"`
	File     string `json:"file"`
	Title    string `json:"title"`
	Section  string `json:"section"`
	Order    int    `json:"order"`
	Summary  string `json:"summary"`
	Reviewed string `json:"reviewed"`
	Body     string `json:"body"`
}

var all []Article
var loadErr error

func init() { all, loadErr = load(files) }

// All returns every article, sorted by section and then order. It returns an
// error if any file has a bad header, so a broken article fails tests and
// start-up checks instead of silently disappearing.
func All() ([]Article, error) { return all, loadErr }

// Get returns one article by slug.
func Get(slug string) (Article, bool) {
	for _, a := range all {
		if a.Slug == slug {
			return a, true
		}
	}
	return Article{}, false
}

func sectionRank(s string) int {
	for i, x := range Sections {
		if x == s {
			return i
		}
	}
	return len(Sections)
}

func load(fsys fs.FS) ([]Article, error) {
	names, err := fs.Glob(fsys, "*.md")
	if err != nil {
		return nil, err
	}
	out := []Article{}
	for _, name := range names {
		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		a, err := parse(name, string(b))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		out = append(out, a)
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := sectionRank(out[i].Section), sectionRank(out[j].Section)
		if ri != rj {
			return ri < rj
		}
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].Title < out[j].Title
	})
	return out, nil
}

func parse(name, text string) (Article, error) {
	a := Article{File: "docs/kb/" + name, Slug: strings.TrimSuffix(path.Base(name), ".md")}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return a, fmt.Errorf("missing header")
	}
	head, body, ok := strings.Cut(text[4:], "\n---\n")
	if !ok {
		return a, fmt.Errorf("header not closed with ---")
	}
	for _, line := range strings.Split(head, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch strings.TrimSpace(k) {
		case "title":
			a.Title = v
		case "section":
			a.Section = v
		case "order":
			n, err := strconv.Atoi(v)
			if err != nil {
				return a, fmt.Errorf("order %q is not a number", v)
			}
			a.Order = n
		case "summary":
			a.Summary = v
		case "reviewed":
			a.Reviewed = v
		}
	}
	if a.Title == "" || a.Section == "" || a.Summary == "" || a.Reviewed == "" {
		return a, fmt.Errorf("header needs title, section, summary and reviewed")
	}
	if sectionRank(a.Section) == len(Sections) {
		return a, fmt.Errorf("unknown section %q (use one of %s)", a.Section, strings.Join(Sections, ", "))
	}
	a.Body = strings.TrimLeft(body, "\n")
	return a, nil
}
