package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"nagisa/internal/biz"
	"nagisa/internal/conf"
	"nagisa/internal/data/ent"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/go-kratos/kratos/v3/log"
	"github.com/google/wire"

	_ "github.com/go-sql-driver/mysql"
	_ "modernc.org/sqlite"
)

// ProviderSet is data providers.
var ProviderSet = wire.NewSet(
	NewData,
	NewObjectStore,
	NewUserRepo,
	NewNodeRepo,
	NewAclRepo,
	NewFileRepo,
	NewShareRepo,
	NewAuditRepo,
	NewSettingRepo,
	NewStatsRepo,
	NewSeeder,
	// The data layer owns the concrete transaction manager, so the binding to
	// the domain interface lives here rather than in the command.
	wire.Bind(new(biz.TxManager), new(*Data)),
)

type txKey struct{}

// Data holds the long-lived storage clients shared by every repository.
//
// Write transactions are serialised by a process level mutex. SQLite allows a
// single writer at a time, and holding the mutex means the read-then-write
// sequences that keep the tree consistent, such as a name collision check
// followed by an insert, can never interleave with another writer.
type Data struct {
	db      *ent.Client
	sqlDB   *sql.DB
	objects *objectStore
	cfg     *conf.Data
	writeMu sync.Mutex
}

// DataOptions carries the resolved configuration for the storage layer.
type DataOptions struct {
	Driver          string
	Source          string
	Debug           bool
	AutoMigrate     bool
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	WAL             bool
	BusyTimeout     time.Duration
	ForeignKeys     bool
}

// NewData opens the database client and returns it with a cleanup function.
func NewData(c *conf.Data) (*Data, func(), error) {
	dc := c.GetDatabase()
	opts := DataOptions{
		Driver:          dc.GetDriver(),
		Source:          dc.GetSource(),
		Debug:           dc.GetDebug(),
		AutoMigrate:     dc.GetAutoMigrate(),
		MaxOpenConns:    int(dc.GetMaxOpenConns()),
		MaxIdleConns:    int(dc.GetMaxIdleConns()),
		WAL:             dc.GetWal(),
		ForeignKeys:     dc.GetForeignKeys(),
		BusyTimeout:     dc.GetBusyTimeout().AsDuration(),
		ConnMaxLifetime: dc.GetConnMaxLifetime().AsDuration(),
	}
	if opts.Driver == "" {
		opts.Driver = "sqlite"
	}
	if opts.BusyTimeout <= 0 {
		opts.BusyTimeout = 10 * time.Second
	}
	return openData(opts, c)
}

func openData(opts DataOptions, c *conf.Data) (*Data, func(), error) {
	var (
		db    *ent.Client
		sqlDB *sql.DB
		err   error
	)
	switch normalizedDriver(opts.Driver) {
	case "sqlite":
		db, sqlDB, err = openSQLite(opts)
	default:
		db, err = ent.Open(opts.Driver, opts.Source)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("data: open database: %w", err)
	}
	if opts.Debug {
		db = db.Debug()
	}
	if opts.AutoMigrate {
		if err := db.Schema.Create(context.Background()); err != nil {
			_ = db.Close()
			return nil, nil, fmt.Errorf("data: migrate schema: %w", err)
		}
	}
	objects, err := newObjectStore(c)
	if err != nil {
		_ = db.Close()
		return nil, nil, err
	}
	d := &Data{db: db, sqlDB: sqlDB, objects: objects, cfg: c}
	cleanup := func() {
		log.Info("closing the data resources")
		if err := db.Close(); err != nil {
			log.Error("failed closing the database", "err", err)
		}
	}
	return d, cleanup, nil
}

// openSQLite opens a modernc.org/sqlite database through the ent SQLite
// dialect so the pragmas the server depends on can be applied per connection.
func openSQLite(opts DataOptions) (*ent.Client, *sql.DB, error) {
	dsn := opts.Source
	if dsn == "" {
		dsn = "file:nagisa.db?cache=shared"
	}
	dir := filepath.Dir(dsn)
	if !strings.HasPrefix(dsn, "file:") && dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, nil, fmt.Errorf("data: create database directory: %w", err)
		}
	}
	dsn = applySQLitePragmas(dsn, opts)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("data: open sqlite: %w", err)
	}
	maxOpen := opts.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = 8
	}
	db.SetMaxOpenConns(maxOpen)
	idle := opts.MaxIdleConns
	if idle <= 0 {
		idle = maxOpen
	}
	db.SetMaxIdleConns(idle)
	if opts.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(opts.ConnMaxLifetime)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("data: ping sqlite: %w", err)
	}
	drv := entsql.OpenDB(dialect.SQLite, db)
	return ent.NewClient(ent.Driver(drv)), db, nil
}

// applySQLitePragmas adds the pragmas the server depends on for durability and
// concurrency, without duplicating ones the operator already configured.
func applySQLitePragmas(dsn string, opts DataOptions) string {
	pragmas := make([]string, 0, 4)
	if opts.WAL && !strings.Contains(dsn, "journal_mode") {
		pragmas = append(pragmas, "_pragma=journal_mode(WAL)")
	}
	if !strings.Contains(dsn, "busy_timeout") {
		pragmas = append(pragmas, fmt.Sprintf("_pragma=busy_timeout(%d)", opts.BusyTimeout.Milliseconds()))
	}
	if opts.ForeignKeys && !strings.Contains(dsn, "foreign_keys") {
		pragmas = append(pragmas, "_pragma=foreign_keys(1)")
	}
	if !strings.Contains(dsn, "synchronous") {
		pragmas = append(pragmas, "_pragma=synchronous(NORMAL)")
	}
	// Beginning a transaction in IMMEDIATE mode takes the write lock up
	// front. A deferred transaction that reads and then writes has to upgrade
	// its lock, and SQLite refuses that upgrade immediately instead of
	// honouring busy_timeout, which is the classic source of "database is
	// locked" under concurrent writers.
	if !strings.Contains(dsn, "_txlock") {
		pragmas = append(pragmas, "_txlock=immediate")
	}
	if len(pragmas) == 0 {
		return dsn
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + strings.Join(pragmas, "&")
}

func normalizedDriver(driver string) string {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "sqlite", "sqlite3", "modernc", "modernc.org/sqlite":
		return "sqlite"
	default:
		return driver
	}
}

// Client returns the ent client for read paths.
func (d *Data) Client() *ent.Client { return d.db }

// Objects returns the object storage client.
func (d *Data) Objects() *objectStore { return d.objects }

// SqlDB returns the underlying database handle, which the health check pings.
func (d *Data) SqlDB() *sql.DB { return d.sqlDB }

// WithTx runs fn inside a single write transaction. Repositories called with
// the context it passes down join that transaction, so a usecase may span
// several repositories and still commit atomically. A nested call joins the
// enclosing transaction instead of opening a second one.
func (d *Data) WithTx(ctx context.Context, fn func(context.Context) error) error {
	if _, ok := txFromContext(ctx); ok {
		return fn(ctx)
	}
	d.writeMu.Lock()
	defer d.writeMu.Unlock()

	tx, err := d.db.Tx(ctx)
	if err != nil {
		return fmt.Errorf("data: begin transaction: %w", err)
	}
	if err := fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		if rerr := tx.Rollback(); rerr != nil && !errors.Is(rerr, sql.ErrTxDone) {
			log.Error("data: rollback failed", "err", rerr)
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("data: commit transaction: %w", err)
	}
	return nil
}

// txFromContext returns the transaction bound to the context, when there is
// one.
func txFromContext(ctx context.Context) (*ent.Tx, bool) {
	tx, ok := ctx.Value(txKey{}).(*ent.Tx)
	return tx, ok && tx != nil
}

// exec bundles the ent accessors so a repository can run against either the
// shared client or the transaction bound to the context without branching at
// every call site.
type exec struct {
	client *ent.Client
	tx     *ent.Tx
}

// Exec returns the executor for the current context.
func (d *Data) Exec(ctx context.Context) exec {
	if tx, ok := txFromContext(ctx); ok {
		return exec{tx: tx}
	}
	return exec{client: d.db}
}

func (e exec) User() *ent.UserClient {
	if e.tx != nil {
		return e.tx.User
	}
	return e.client.User
}

func (e exec) Node() *ent.NodeClient {
	if e.tx != nil {
		return e.tx.Node
	}
	return e.client.Node
}

func (e exec) NodeAcl() *ent.NodeAclClient {
	if e.tx != nil {
		return e.tx.NodeAcl
	}
	return e.client.NodeAcl
}

func (e exec) Upload() *ent.UploadClient {
	if e.tx != nil {
		return e.tx.Upload
	}
	return e.client.Upload
}

func (e exec) UploadPart() *ent.UploadPartClient {
	if e.tx != nil {
		return e.tx.UploadPart
	}
	return e.client.UploadPart
}

func (e exec) NodeVersion() *ent.NodeVersionClient {
	if e.tx != nil {
		return e.tx.NodeVersion
	}
	return e.client.NodeVersion
}

func (e exec) PendingDeletion() *ent.PendingDeletionClient {
	if e.tx != nil {
		return e.tx.PendingDeletion
	}
	return e.client.PendingDeletion
}

func (e exec) Share() *ent.ShareClient {
	if e.tx != nil {
		return e.tx.Share
	}
	return e.client.Share
}

func (e exec) RefreshSession() *ent.RefreshSessionClient {
	if e.tx != nil {
		return e.tx.RefreshSession
	}
	return e.client.RefreshSession
}

func (e exec) Setting() *ent.SettingClient {
	if e.tx != nil {
		return e.tx.Setting
	}
	return e.client.Setting
}

func (e exec) AuditLog() *ent.AuditLogClient {
	if e.tx != nil {
		return e.tx.AuditLog
	}
	return e.client.AuditLog
}

// InTx reports whether the executor runs inside a transaction.
func (e exec) InTx() bool { return e.tx != nil }

// Logger returns a repository scoped logger.
func (d *Data) Logger() *slog.Logger { return slog.Default() }

// escapeLike escapes the LIKE wildcards of a user supplied value.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "%", `\%`)
	s = strings.ReplaceAll(s, "_", `\_`)
	return s
}
