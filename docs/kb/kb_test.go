package kb

import (
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestArticlesLoad(t *testing.T) {
	list, err := All()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) == 0 {
		t.Fatal("no articles")
	}
	seen := map[string]bool{}
	for _, a := range list {
		if seen[a.Slug] {
			t.Errorf("duplicate slug %s", a.Slug)
		}
		seen[a.Slug] = true
		if _, err := time.Parse("2006-01-02", a.Reviewed); err != nil {
			t.Errorf("%s: reviewed %q is not YYYY-MM-DD", a.File, a.Reviewed)
		}
		if strings.HasPrefix(a.Body, "# ") {
			t.Errorf("%s: don't start with a # heading; the title comes from the header", a.File)
		}
	}
}

// Links like [text](other.md) or (other.md#part) must point at an article
// that exists.
var mdLink = regexp.MustCompile(`\]\(([^)\s]+?\.md)(#[^)]*)?\)`)

func TestLinksResolve(t *testing.T) {
	list, _ := All()
	for _, a := range list {
		for _, m := range mdLink.FindAllStringSubmatch(a.Body, -1) {
			target := m[1]
			if strings.Contains(target, "/") {
				continue // a repo file such as ../API.md, not an article
			}
			if _, ok := Get(strings.TrimSuffix(target, ".md")); !ok {
				t.Errorf("%s links to missing article %s", a.File, target)
			}
		}
	}
}

func TestParseRejectsBadHeaders(t *testing.T) {
	cases := map[string]string{
		"none.md":    "## Hi\n",
		"open.md":    "---\ntitle: x\n",
		"missing.md": "---\ntitle: x\nsection: FAQ\n---\nbody",
		"section.md": "---\ntitle: x\nsection: Misc\nsummary: s\nreviewed: 2026-01-01\n---\nbody",
		"order.md":   "---\ntitle: x\nsection: FAQ\norder: first\nsummary: s\nreviewed: 2026-01-01\n---\nbody",
	}
	for name, text := range cases {
		if _, err := load(fstest.MapFS{name: {Data: []byte(text)}}); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	got, err := load(fstest.MapFS{
		"b.md": {Data: []byte("---\ntitle: B\nsection: FAQ\norder: 10\nsummary: s\nreviewed: 2026-01-01\n---\n\n## Body\n")},
		"a.md": {Data: []byte("---\ntitle: A\nsection: Start here\norder: 20\nsummary: s\nreviewed: 2026-01-01\n---\ntext")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Slug != "a" || got[1].Slug != "b" || got[1].Body != "## Body\n" || got[0].File != "docs/kb/a.md" {
		t.Fatalf("unexpected %+v", got)
	}
}
