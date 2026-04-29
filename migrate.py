#!/usr/bin/env python3
"""
Import YOURLS MySQL dump into the u URL-shortener SQLite database.

Usage:
    python3 migrate.py --dump dump.sql --db u.db

What gets imported:
    yourls_url  → links  (keyword, url, title, timestamp→created_at, ip, clicks)
    yourls_log  → clicks (click_id→id, click_time→clicked_at, shorturl→keyword,
                           referrer, user_agent, ip_address→ip)
    counter.next_val  set to max(numeric keyword) + 1

Skips:
    - yourls_options (app config)
    - click records whose shorturl has no matching link
"""

import argparse
import re
import sqlite3
import sys


# ---------------------------------------------------------------------------
# MySQL value-list parser
# ---------------------------------------------------------------------------

def parse_values(values_sql: str) -> list[tuple]:
    """
    Parse MySQL multi-row INSERT values string, e.g.:
        (1,'foo','bar\'s'),(2,'baz',NULL)
    Returns a list of tuples where each element is a Python str, int, float,
    or None.
    """
    rows = []
    i = 0
    n = len(values_sql)

    while i < n:
        # Skip whitespace / commas between tuples
        while i < n and values_sql[i] in (' ', '\t', '\n', ','):
            i += 1
        if i >= n:
            break
        if values_sql[i] != '(':
            i += 1
            continue

        # Consume one tuple
        i += 1  # skip '('
        fields = []
        while i < n and values_sql[i] != ')':
            # Skip whitespace / commas between fields
            while i < n and values_sql[i] in (' ', '\t', ','):
                i += 1
            if i >= n or values_sql[i] == ')':
                break

            ch = values_sql[i]

            if ch == "'":
                # String literal with MySQL escapes
                i += 1
                buf = []
                while i < n:
                    c = values_sql[i]
                    if c == '\\' and i + 1 < n:
                        nxt = values_sql[i + 1]
                        escape_map = {
                            "'": "'", '\\': '\\', 'n': '\n',
                            'r': '\r', 't': '\t', '0': '\x00',
                        }
                        buf.append(escape_map.get(nxt, nxt))
                        i += 2
                    elif c == "'" and i + 1 < n and values_sql[i + 1] == "'":
                        # doubled single-quote
                        buf.append("'")
                        i += 2
                    elif c == "'":
                        i += 1
                        break
                    else:
                        buf.append(c)
                        i += 1
                fields.append(''.join(buf))

            elif ch in '-0123456789':
                # Number
                j = i
                if ch == '-':
                    i += 1
                while i < n and (values_sql[i].isdigit() or values_sql[i] == '.'):
                    i += 1
                num_str = values_sql[j:i]
                fields.append(float(num_str) if '.' in num_str else int(num_str))

            elif values_sql[i:i+4].upper() == 'NULL':
                fields.append(None)
                i += 4

            else:
                # Unknown token — skip to next comma or closing paren
                while i < n and values_sql[i] not in (',', ')'):
                    i += 1

        rows.append(tuple(fields))
        # Skip the closing ')'
        if i < n and values_sql[i] == ')':
            i += 1

    return rows


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    ap = argparse.ArgumentParser(description="Import YOURLS dump into u SQLite DB")
    ap.add_argument("--dump", default="dump.sql", help="path to MySQL dump file")
    ap.add_argument("--db",   default="u.db",    help="path to SQLite database")
    args = ap.parse_args()

    # Read dump
    print(f"Reading {args.dump} …")
    with open(args.dump, encoding="utf-8", errors="replace") as f:
        content = f.read()

    # Extract INSERT blocks for the two tables we care about
    url_insert = _extract_insert(content, "yourls_url")
    log_insert = _extract_insert(content, "yourls_log")

    if not url_insert:
        print("WARNING: no INSERT for yourls_url found — nothing to import")

    # Open SQLite and ensure schema exists
    con = sqlite3.connect(args.db)
    con.execute("PRAGMA journal_mode=WAL")
    con.execute("PRAGMA foreign_keys=OFF")   # we handle FK manually
    _ensure_schema(con)

    # ---- Import links -------------------------------------------------------
    link_count = 0
    known_keywords: set[str] = set()

    if url_insert:
        print("Parsing yourls_url …")
        rows = parse_values(url_insert)
        print(f"  Found {len(rows)} rows")

        with con:
            for row in rows:
                if len(row) < 6:
                    continue
                keyword, url, title, timestamp, ip, clicks = (
                    row[0], row[1],
                    row[2] or '',
                    row[3] or '',
                    row[4] or '',
                    int(row[5]) if row[5] is not None else 0,
                )
                con.execute(
                    """INSERT OR IGNORE INTO links
                       (keyword, url, title, created_at, ip, clicks)
                       VALUES (?, ?, ?, ?, ?, ?)""",
                    (keyword, url, title, timestamp, ip, clicks),
                )
                known_keywords.add(keyword)
                link_count += 1

    print(f"Imported {link_count} links.")

    # ---- Import clicks -------------------------------------------------------
    click_count = 0
    skipped_clicks = 0

    if log_insert:
        print("Parsing yourls_log …")
        rows = parse_values(log_insert)
        print(f"  Found {len(rows)} rows")

        BATCH = 1000
        batch = []

        def flush():
            nonlocal click_count
            with con:
                con.executemany(
                    """INSERT OR IGNORE INTO clicks
                       (id, keyword, clicked_at, referrer, user_agent, ip)
                       VALUES (?, ?, ?, ?, ?, ?)""",
                    batch,
                )
            click_count += len(batch)
            batch.clear()

        for row in rows:
            if len(row) < 6:
                continue
            # (click_id, click_time, shorturl, referrer, user_agent, ip_address, country_code)
            click_id   = int(row[0])
            click_time = row[1] or ''
            shorturl   = row[2] or ''
            referrer   = row[3] or ''
            user_agent = row[4] or ''
            ip_address = row[5] or ''
            # row[6] = country_code — not stored

            if shorturl not in known_keywords:
                skipped_clicks += 1
                continue

            batch.append((click_id, shorturl, click_time, referrer, user_agent, ip_address))
            if len(batch) >= BATCH:
                flush()

        if batch:
            flush()

    print(f"Imported {click_count} clicks ({skipped_clicks} skipped — no matching link).")

    # ---- Update counter ------------------------------------------------------
    max_numeric = 0
    cur = con.execute("SELECT keyword FROM links")
    for (kw,) in cur.fetchall():
        try:
            val = _base36_decode(kw)
            if val > max_numeric:
                max_numeric = val
        except ValueError:
            pass  # custom keyword, not auto-generated

    next_val = max_numeric + 1
    with con:
        con.execute(
            "INSERT OR REPLACE INTO counter (id, next_val) VALUES (1, ?)",
            (next_val,),
        )
    print(f"Counter set to {next_val} (max numeric keyword was {max_numeric}).")

    con.close()
    print("Done.")


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _extract_insert(content: str, table: str) -> str | None:
    """Return the combined VALUES(...) portion of all INSERTs for the given table.

    Each INSERT is on its own line in a MariaDB dump, so we collect full lines
    rather than using a regex that can be tripped up by semicolons inside
    quoted strings (e.g., user-agent values like 'Mozilla/5.0; ...')
    """
    prefix = f"INSERT INTO `{table}` VALUES "
    blocks = []
    for line in content.splitlines():
        if line.startswith(prefix):
            # Strip the prefix and the trailing semicolon
            values_part = line[len(prefix):]
            if values_part.endswith(";"):
                values_part = values_part[:-1]
            blocks.append(values_part.strip())
    if not blocks:
        return None
    return ",".join(blocks)


def _ensure_schema(con: sqlite3.Connection):
    con.executescript("""
        CREATE TABLE IF NOT EXISTS links (
            keyword    TEXT PRIMARY KEY,
            url        TEXT NOT NULL,
            title      TEXT NOT NULL DEFAULT '',
            created_at TEXT NOT NULL DEFAULT (datetime('now')),
            ip         TEXT NOT NULL DEFAULT '',
            clicks     INTEGER NOT NULL DEFAULT 0
        );

        CREATE TABLE IF NOT EXISTS clicks (
            id         INTEGER PRIMARY KEY,
            keyword    TEXT NOT NULL,
            clicked_at TEXT NOT NULL DEFAULT (datetime('now')),
            referrer   TEXT NOT NULL DEFAULT '',
            user_agent TEXT NOT NULL DEFAULT '',
            ip         TEXT NOT NULL DEFAULT ''
        );

        CREATE TABLE IF NOT EXISTS counter (
            id       INTEGER PRIMARY KEY CHECK (id = 1),
            next_val INTEGER NOT NULL DEFAULT 1
        );
        INSERT OR IGNORE INTO counter (id, next_val) VALUES (1, 1);
    """)


_BASE36 = "0123456789abcdefghijklmnopqrstuvwxyz"
_SQLITE_INT_MAX = (1 << 63) - 1

def _base36_decode(s: str) -> int:
    """Decode base36 string to int. Raises ValueError if not valid base36
    or if the result exceeds SQLite's INTEGER range (custom keywords)."""
    s = s.lower()
    result = 0
    for ch in s:
        if ch not in _BASE36:
            raise ValueError(f"not base36: {s!r}")
        result = result * 36 + _BASE36.index(ch)
        if result > _SQLITE_INT_MAX:
            raise ValueError(f"base36 overflow (custom keyword): {s!r}")
    return result


if __name__ == "__main__":
    main()
