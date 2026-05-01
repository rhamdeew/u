package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Link struct {
	Keyword      string
	URL          string
	Title        string
	CreatedAt    time.Time
	IP           string
	Clicks       int
	CategoryID   int
	CategoryName string
}

type ListOpts struct {
	Search     string
	SortBy     string // keyword | url | title | created_at | clicks
	SortDesc   bool
	Page       int
	PerPage    int
	CategoryID int
}

type ListResult struct {
	Links      []Link
	Total      int
	TotalPages int
}

const linkSelect = `
	SELECT l.keyword, l.url, l.title, l.created_at, l.ip, l.clicks,
	       COALESCE(l.category_id, 0), COALESCE(c.name, '')
	FROM links l LEFT JOIN categories c ON c.id = l.category_id`

func scanLink(row interface {
	Scan(...any) error
}) (Link, string, error) {
	var l Link
	var createdAt string
	err := row.Scan(&l.Keyword, &l.URL, &l.Title, &createdAt, &l.IP, &l.Clicks,
		&l.CategoryID, &l.CategoryName)
	return l, createdAt, err
}

// GetByKeyword returns the link or nil if not found.
func (d *DB) GetByKeyword(keyword string) (*Link, error) {
	row := d.sql.QueryRow(linkSelect+` WHERE l.keyword = ?`, keyword)
	l, createdAt, err := scanLink(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	l.CreatedAt = parseTime(createdAt)
	return &l, nil
}

// URLExists returns the existing link for a long URL, or nil if not found.
func (d *DB) URLExists(url string) (*Link, error) {
	row := d.sql.QueryRow(linkSelect+` WHERE l.url = ? LIMIT 1`, url)
	l, createdAt, err := scanLink(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	l.CreatedAt = parseTime(createdAt)
	return &l, nil
}

// Insert stores a new link. If l.Keyword is empty, one is auto-generated via the counter.
func (d *DB) Insert(l *Link) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if l.Keyword == "" {
		kw, err := nextKeyword(tx)
		if err != nil {
			return fmt.Errorf("generating keyword: %w", err)
		}
		l.Keyword = kw
	}

	if l.CreatedAt.IsZero() {
		l.CreatedAt = time.Now().UTC()
	}

	var catID any
	if l.CategoryID > 0 {
		catID = l.CategoryID
	}

	_, err = tx.Exec(
		`INSERT INTO links (keyword, url, title, created_at, ip, clicks, category_id)
		 VALUES (?, ?, ?, ?, ?, 0, ?)`,
		l.Keyword, l.URL, l.Title, l.CreatedAt.Format("2006-01-02 15:04:05"), l.IP, catID,
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// Update modifies an existing link. If newKeyword is empty, the keyword is unchanged.
func (d *DB) Update(oldKeyword, newKeyword, url, title string, categoryID int) error {
	if newKeyword == "" {
		newKeyword = oldKeyword
	}
	var catID any
	if categoryID > 0 {
		catID = categoryID
	}
	_, err := d.sql.Exec(
		`UPDATE links SET keyword = ?, url = ?, title = ?, category_id = ? WHERE keyword = ?`,
		newKeyword, url, title, catID, oldKeyword,
	)
	return err
}

// IncrClicks atomically increments the click counter for a keyword.
func (d *DB) IncrClicks(keyword string) error {
	_, err := d.sql.Exec(`UPDATE links SET clicks = clicks + 1 WHERE keyword = ?`, keyword)
	return err
}

// Delete removes a link and all its click records (via ON DELETE CASCADE).
func (d *DB) Delete(keyword string) error {
	_, err := d.sql.Exec(`DELETE FROM links WHERE keyword = ?`, keyword)
	return err
}

// List returns a paginated, filtered, sorted list of links.
func (d *DB) List(opts ListOpts) (ListResult, error) {
	if opts.Page < 1 {
		opts.Page = 1
	}
	if opts.PerPage < 1 {
		opts.PerPage = 20
	}

	validSort := map[string]bool{
		"keyword": true, "url": true, "title": true,
		"created_at": true, "clicks": true,
	}
	if !validSort[opts.SortBy] {
		opts.SortBy = "created_at"
	}
	dir := "ASC"
	if opts.SortDesc {
		dir = "DESC"
	}

	where, args := buildWhere(opts.Search, opts.CategoryID)

	var total int
	countQ := "SELECT COUNT(*) FROM links l LEFT JOIN categories c ON c.id = l.category_id" + where
	if err := d.sql.QueryRow(countQ, args...).Scan(&total); err != nil {
		return ListResult{}, err
	}

	totalPages := (total + opts.PerPage - 1) / opts.PerPage
	if totalPages == 0 {
		totalPages = 1
	}
	offset := (opts.Page - 1) * opts.PerPage

	query := fmt.Sprintf(
		linkSelect+`%s ORDER BY l.%s %s LIMIT ? OFFSET ?`,
		where, opts.SortBy, dir,
	)
	queryArgs := append(args, opts.PerPage, offset)

	rows, err := d.sql.Query(query, queryArgs...)
	if err != nil {
		return ListResult{}, err
	}
	defer rows.Close()

	var links []Link
	for rows.Next() {
		l, createdAt, err := scanLink(rows)
		if err != nil {
			return ListResult{}, err
		}
		l.CreatedAt = parseTime(createdAt)
		links = append(links, l)
	}
	if err := rows.Err(); err != nil {
		return ListResult{}, err
	}

	return ListResult{
		Links:      links,
		Total:      total,
		TotalPages: totalPages,
	}, nil
}

// TotalStats returns the total number of links and clicks.
func (d *DB) TotalStats() (links, clicks int, err error) {
	err = d.sql.QueryRow(`SELECT COUNT(*), COALESCE(SUM(clicks), 0) FROM links`).Scan(&links, &clicks)
	return
}

// buildWhere returns a WHERE clause and args for search + optional category filter.
func buildWhere(search string, categoryID int) (string, []any) {
	var clauses []string
	var args []any

	if search != "" {
		s := "%" + search + "%"
		clauses = append(clauses, "(l.keyword LIKE ? OR l.url LIKE ? OR l.title LIKE ?)")
		args = append(args, s, s, s)
	}
	if categoryID > 0 {
		clauses = append(clauses, "l.category_id = ?")
		args = append(args, categoryID)
	}

	if len(clauses) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

// nextKeyword atomically increments the counter and returns a base36 keyword.
// Must be called inside a transaction.
func nextKeyword(tx *sql.Tx) (string, error) {
	var n int64
	if err := tx.QueryRow(`SELECT next_val FROM counter WHERE id = 1`).Scan(&n); err != nil {
		return "", err
	}
	if _, err := tx.Exec(`UPDATE counter SET next_val = next_val + 1 WHERE id = 1`); err != nil {
		return "", err
	}
	return EncodeBase36(n), nil
}

// EncodeBase36 converts an integer to a base-36 string (digits + lowercase letters).
func EncodeBase36(n int64) string {
	const charset = "0123456789abcdefghijklmnopqrstuvwxyz"
	if n == 0 {
		return "0"
	}
	var buf []byte
	for n > 0 {
		buf = append([]byte{charset[n%36]}, buf...)
		n /= 36
	}
	return string(buf)
}

func parseTime(s string) time.Time {
	layouts := []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05Z", time.RFC3339}
	for _, l := range layouts {
		if t, err := time.ParseInLocation(l, s, time.UTC); err == nil {
			return t
		}
	}
	return time.Time{}
}
