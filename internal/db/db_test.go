package db_test

import (
	"os"
	"testing"

	"u/internal/db"
)

func TestEncodeBase36(t *testing.T) {
	cases := []struct {
		in  int64
		out string
	}{
		{0, "0"},
		{1, "1"},
		{35, "z"},
		{36, "10"},
		{37, "11"},
		{1296, "100"}, // 36^2
	}
	for _, c := range cases {
		got := db.EncodeBase36(c.in)
		if got != c.out {
			t.Errorf("EncodeBase36(%d) = %q, want %q", c.in, got, c.out)
		}
	}
}

func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	d, err := db.Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestInsertAndGet(t *testing.T) {
	d := openTestDB(t)

	link := &db.Link{URL: "https://example.com", Title: "Example"}
	if err := d.Insert(link); err != nil {
		t.Fatal(err)
	}
	if link.Keyword == "" {
		t.Error("keyword should have been auto-generated")
	}

	got, err := d.GetByKeyword(link.Keyword)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected link, got nil")
	}
	if got.URL != "https://example.com" {
		t.Errorf("URL mismatch: got %q", got.URL)
	}
}

func TestInsertCustomKeyword(t *testing.T) {
	d := openTestDB(t)

	link := &db.Link{Keyword: "hello", URL: "https://example.com"}
	if err := d.Insert(link); err != nil {
		t.Fatal(err)
	}

	got, _ := d.GetByKeyword("hello")
	if got == nil || got.URL != "https://example.com" {
		t.Error("custom keyword not stored correctly")
	}
}

func TestGetByKeywordNotFound(t *testing.T) {
	d := openTestDB(t)
	got, err := d.GetByKeyword("nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Error("expected nil for missing keyword")
	}
}

func TestUpdate(t *testing.T) {
	d := openTestDB(t)

	link := &db.Link{Keyword: "old", URL: "https://old.com"}
	_ = d.Insert(link)

	if err := d.Update("old", "new", "https://new.com", "New Title", 0); err != nil {
		t.Fatal(err)
	}

	old, _ := d.GetByKeyword("old")
	if old != nil {
		t.Error("old keyword should not exist after rename")
	}
	newLink, _ := d.GetByKeyword("new")
	if newLink == nil || newLink.URL != "https://new.com" {
		t.Error("new keyword not found or wrong URL")
	}
}

func TestDelete(t *testing.T) {
	d := openTestDB(t)

	link := &db.Link{Keyword: "todel", URL: "https://example.com"}
	_ = d.Insert(link)
	_ = d.Delete("todel")

	got, _ := d.GetByKeyword("todel")
	if got != nil {
		t.Error("link should be deleted")
	}
}

func TestIncrClicks(t *testing.T) {
	d := openTestDB(t)

	_ = d.Insert(&db.Link{Keyword: "clk", URL: "https://example.com"})
	_ = d.IncrClicks("clk")
	_ = d.IncrClicks("clk")

	got, _ := d.GetByKeyword("clk")
	if got.Clicks != 2 {
		t.Errorf("expected 2 clicks, got %d", got.Clicks)
	}
}

func TestList(t *testing.T) {
	d := openTestDB(t)

	for i := 0; i < 5; i++ {
		_ = d.Insert(&db.Link{URL: "https://example.com/page"})
	}

	result, err := d.List(db.ListOpts{Page: 1, PerPage: 3})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 5 {
		t.Errorf("expected total 5, got %d", result.Total)
	}
	if len(result.Links) != 3 {
		t.Errorf("expected 3 links on page 1, got %d", len(result.Links))
	}
	if result.TotalPages != 2 {
		t.Errorf("expected 2 pages, got %d", result.TotalPages)
	}
}

func TestListSearch(t *testing.T) {
	d := openTestDB(t)

	_ = d.Insert(&db.Link{URL: "https://golang.org", Title: "Go"})
	_ = d.Insert(&db.Link{URL: "https://rust-lang.org", Title: "Rust"})

	result, _ := d.List(db.ListOpts{Search: "golang", Page: 1, PerPage: 20})
	if result.Total != 1 {
		t.Errorf("expected 1 result for 'golang', got %d", result.Total)
	}
}

func TestAutoKeywordSequence(t *testing.T) {
	d := openTestDB(t)

	var keywords []string
	for i := 0; i < 5; i++ {
		l := &db.Link{URL: "https://example.com"}
		_ = d.Insert(l)
		keywords = append(keywords, l.Keyword)
	}

	// All keywords should be unique
	seen := map[string]bool{}
	for _, k := range keywords {
		if seen[k] {
			t.Errorf("duplicate keyword: %s", k)
		}
		seen[k] = true
	}
}
