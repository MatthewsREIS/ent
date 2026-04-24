// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package edgeschema

// SQL snapshot regression harness.
//
// Captures SQL emitted by representative ent operations against a sqlite in-memory
// database and compares against frozen snapshot files on disk.
//
// A failing snapshot means codegen output changed the SQL for a canonical
// operation — either a bug or a deliberate change that needs a matching snapshot
// update (run with -update-sql-snapshots).
//
// Scope: this is a backstop for Phase 3's where.tmpl rewrite. It is NOT intended
// as comprehensive SQL coverage — that's the integration suite's job.
//
// NOTE on file placement: the snapshot files live under entc/gen/testdata/sql_snapshots/
// (two levels up from this file's package directory) so they are co-located with the
// codegen output they guard. The test file itself lives here because
// entc/integration is a separate Go module that owns the edgeschema ent client;
// the root module (entgo.io/ent, where entc/gen lives) cannot import it.

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/entc/integration/edgeschema/ent"
	"entgo.io/ent/entc/integration/edgeschema/ent/migrate"
)

// dbSeq provides unique database names so each test gets an isolated in-memory DB.
var dbSeq atomic.Int64

var updateSnapshots = flag.Bool("update-sql-snapshots", false,
	"rewrite SQL snapshot files to match current output")

// snapshotDir returns the absolute path to entc/gen/testdata/sql_snapshots/,
// derived from this source file's location so it is robust to working-directory changes.
func snapshotDir() string {
	_, thisFile, _, _ := runtime.Caller(0)
	// thisFile is …/entc/integration/edgeschema/sqlsnapshot_test.go
	// go up three levels to reach the repo root, then descend into entc/gen/testdata/sql_snapshots
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	return filepath.Join(repoRoot, "entc", "gen", "testdata", "sql_snapshots")
}

// captureBuf collects normalized SQL statements emitted through the debug driver.
type captureBuf struct {
	mu    sync.Mutex
	stmts []string
}

func (c *captureBuf) log(_ context.Context, args ...any) {
	for _, a := range args {
		s, ok := a.(string)
		if !ok {
			continue
		}
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		c.mu.Lock()
		c.stmts = append(c.stmts, normalizeSQL(s))
		c.mu.Unlock()
	}
}

func (c *captureBuf) drain() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := append([]string(nil), c.stmts...)
	c.stmts = c.stmts[:0]
	return out
}

// normalizeSQL produces a deterministic representation suitable for snapshot
// comparison. It:
//   - collapses internal whitespace
//   - replaces transaction UUIDs with a fixed placeholder
//   - replaces args=[...] lists with args=[...REDACTED...] so that
//     non-deterministic IDs and timestamps do not cause spurious mismatches
//
// The SQL query text itself (everything before "args=") is left intact;
// this is the part that Phase 3's where.tmpl rewrite would change.
var (
	reTxID   = regexp.MustCompile(`Tx\([0-9a-f-]{36}\)`)
	reArgs   = regexp.MustCompile(`args=\[.*?\]`)
)

func normalizeSQL(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	s = reTxID.ReplaceAllString(s, "Tx(<id>)")
	s = reArgs.ReplaceAllString(s, "args=[<redacted>]")
	return s
}

// snapshot compares got against the snapshot file at <snapshotDir()>/<name>.sql.
// Updates the file in-place if -update-sql-snapshots is set.
func snapshot(t *testing.T, name string, got []string) {
	t.Helper()
	dir := snapshotDir()
	path := filepath.Join(dir, name+".sql")
	want := strings.Join(got, "\n") + "\n"
	if *updateSnapshots {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
			t.Fatalf("write snapshot: %v", err)
		}
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read snapshot %s: %v (run with -update-sql-snapshots to create)", path, err)
	}
	if strings.TrimSpace(string(raw)) != strings.TrimSpace(want) {
		t.Fatalf("snapshot %s mismatch:\n--- want ---\n%s\n--- got  ---\n%s",
			path, string(raw), want)
	}
}

// newCapturingClient opens an in-memory sqlite, wires a debug driver that logs
// every emitted SQL statement to the returned captureBuf, and runs the schema
// migration. Each caller gets an isolated database.
func newCapturingClient(t *testing.T) (*ent.Client, *captureBuf, func()) {
	t.Helper()
	cap := &captureBuf{}
	id := dbSeq.Add(1)
	dsn := fmt.Sprintf("file:snap%d?mode=memory&cache=shared&_fk=1", id)
	drv, err := entsql.Open(dialect.SQLite, dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	debug := dialect.DebugWithContext(drv, cap.log)
	client := ent.NewClient(ent.Driver(debug))
	ctx := context.Background()
	if err := client.Schema.Create(ctx, migrate.WithGlobalUniqueID(true)); err != nil {
		client.Close()
		t.Fatalf("schema create: %v", err)
	}
	cap.drain() // discard DDL; only keep subsequent DML operations
	return client, cap, func() { client.Close() }
}

// TestSnapshot_CreateUserWithTweet covers create.go setter + edge-add path.
// Creates a user, then creates a tweet associated to that user via AddTweetIDs.
//
// Note: the edgeschema Tweet-User relationship is many-to-many through the
// UserTweet junction entity. There is no SetOwner on Tweet; the canonical
// pattern is AddTweetIDs on User (after the sub-package split, entity-taking
// setters were removed — only *ID variants remain).
func TestSnapshot_CreateUserWithTweet(t *testing.T) {
	client, cap, done := newCapturingClient(t)
	defer done()
	ctx := context.Background()

	t1 := client.Tweet.Create().SetText("hello world").SaveX(ctx)
	client.User.Create().SetName("alice").AddTweetIDs(t1.ID).SaveX(ctx)

	got := cap.drain()
	if len(got) == 0 {
		t.Fatal("no SQL captured; debug-driver wiring may be broken")
	}
	snapshot(t, "create_user_with_tweet", got)
}

// TestSnapshot_UserQuery_WithTweets covers query.go With* eager loading.
func TestSnapshot_UserQuery_WithTweets(t *testing.T) {
	client, cap, done := newCapturingClient(t)
	defer done()
	ctx := context.Background()

	t1 := client.Tweet.Create().SetText("one").SaveX(ctx)
	t2 := client.Tweet.Create().SetText("two").SaveX(ctx)
	client.User.Create().SetName("alice").AddTweetIDs(t1.ID, t2.ID).SaveX(ctx)
	cap.drain() // discard setup SQL; only capture the query below

	client.User.Query().WithTweets().AllX(ctx)

	got := cap.drain()
	if len(got) == 0 {
		t.Fatal("no SQL captured")
	}
	snapshot(t, "user_query_with_tweets", got)
}

// TestSnapshot_TweetUpdate_SetText covers update.go field mutation.
// Updates a tweet's text field — exercises the UPDATE ... SET ... WHERE path.
func TestSnapshot_TweetUpdate_SetText(t *testing.T) {
	client, cap, done := newCapturingClient(t)
	defer done()
	ctx := context.Background()

	tw := client.Tweet.Create().SetText("original text").SaveX(ctx)
	cap.drain()

	client.Tweet.UpdateOneID(tw.ID).SetText("updated text").SaveX(ctx)

	got := cap.drain()
	if len(got) == 0 {
		t.Fatal("no SQL captured")
	}
	snapshot(t, "tweet_update_set_text", got)
}

// TestSnapshot_TweetDelete_WhereID covers delete.go predicate path.
func TestSnapshot_TweetDelete_WhereID(t *testing.T) {
	client, cap, done := newCapturingClient(t)
	defer done()
	ctx := context.Background()

	tw := client.Tweet.Create().SetText("to be deleted").SaveX(ctx)
	cap.drain()

	client.Tweet.DeleteOneID(tw.ID).ExecX(ctx)

	got := cap.drain()
	if len(got) == 0 {
		t.Fatal("no SQL captured")
	}
	snapshot(t, "tweet_delete_where_id", got)
}
