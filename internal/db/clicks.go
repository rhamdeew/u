package db

import "time"

type ClickRecord struct {
	Keyword   string
	ClickedAt time.Time
	Referrer  string
	UserAgent string
	IP        string
}

type DayStat struct {
	Date   string
	Clicks int
}

// InsertClick records a click asynchronously-safe (serialized by single-writer SQLite).
func (d *DB) InsertClick(c ClickRecord) error {
	_, err := d.sql.Exec(
		`INSERT INTO clicks (keyword, clicked_at, referrer, user_agent, ip)
		 VALUES (?, ?, ?, ?, ?)`,
		c.Keyword,
		c.ClickedAt.UTC().Format("2006-01-02 15:04:05"),
		c.Referrer, c.UserAgent, c.IP,
	)
	return err
}

// DayStats returns click counts grouped by day for the given keyword (last 60 days).
func (d *DB) DayStats(keyword string) ([]DayStat, error) {
	rows, err := d.sql.Query(`
		SELECT strftime('%Y-%m-%d', clicked_at) AS day, COUNT(*) AS cnt
		FROM clicks
		WHERE keyword = ?
		  AND clicked_at >= datetime('now', '-60 days')
		GROUP BY day
		ORDER BY day DESC
	`, keyword)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []DayStat
	for rows.Next() {
		var s DayStat
		if err := rows.Scan(&s.Date, &s.Clicks); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}
