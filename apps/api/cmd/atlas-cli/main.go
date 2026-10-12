// Command atlas-cli is the operator's handle on an Atlas deployment
// (docs/ATLAS.md §4.1): database migrations and a few operational commands.
//
// It is deliberately small and explicit. Anything it can do should be something
// an operator can also understand and repeat.
//
// Usage:
//
//	atlas-cli migrate up            apply all pending migrations
//	atlas-cli migrate down          roll back one migration
//	atlas-cli migrate status        show applied/pending migrations
//	atlas-cli migrate version       print the current schema version
//	atlas-cli migrate create <name> scaffold a new empty migration
//	atlas-cli outbox hello [msg]    write a core.system.hello.v1 event
//	atlas-cli seed                  write the role/permission catalogue (§3.2, §6.2)
//	atlas-cli keygen                print fresh local secrets (Ed25519 + AES-256)
//	atlas-cli bootstrap             create the first account (a real person)
//	atlas-cli version               print the build version
package main

import (
	"context"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver "pgx" for goose
	"github.com/pressly/goose/v3"

	"github.com/safaristack/atlas/apps/api/db/migrations"
	"github.com/safaristack/atlas/apps/api/internal/auth"
	"github.com/safaristack/atlas/apps/api/internal/outbox"
	"github.com/safaristack/atlas/apps/api/internal/platform/config"
	"github.com/safaristack/atlas/apps/api/internal/platform/database"
	"github.com/safaristack/atlas/apps/api/internal/platform/ed25519jwt"
	"github.com/safaristack/atlas/apps/api/internal/platform/logging"
	"github.com/safaristack/atlas/apps/api/internal/seed"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return errors.New("no command given")
	}

	if os.Getenv("ATLAS_SERVICE_NAME") == "" {
		_ = os.Setenv("ATLAS_SERVICE_NAME", "atlas-cli")
	}

	switch args[0] {
	case "version", "-v", "--version":
		fmt.Printf("atlas-cli %s\n", version)
		return nil
	case "migrate":
		return runMigrate(args[1:])
	case "outbox":
		return runOutbox(args[1:])
	case "seed":
		return runSeed()
	case "keygen":
		return runKeygen()
	case "bootstrap":
		return runBootstrap(args[1:])
	case "help", "-h", "--help":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `atlas-cli — Atlas operations

  migrate up | down | status | version | create <name>
  outbox hello [message]
  seed
  keygen
  bootstrap --email <email> --name <full name> [--phone] [--role <role>] [--scope <scope>]
  version
`)
}

// ── keygen ──────────────────────────────────────────────────

// runKeygen prints fresh local secrets: an Ed25519 key for realtime tokens and
// an AES-256 key for encrypting short-lived secrets. Run it once and paste the
// output into `.env`; without it the API generates ephemeral keys at boot.
func runKeygen() error {
	key, err := ed25519jwt.Generate()
	if err != nil {
		return err
	}
	der, err := x509.MarshalPKCS8PrivateKey(key.SigningKey())
	if err != nil {
		return err
	}
	encryptionKey, err := auth.GenerateEncryptionKey()
	if err != nil {
		return err
	}

	fmt.Printf("ATLAS_REALTIME_PRIVATE_KEY=%s\n", base64.StdEncoding.EncodeToString(der))
	fmt.Printf("ATLAS_ENCRYPTION_KEY=%s\n", encryptionKey)
	fmt.Printf("# realtime key id (public half): %s\n", key.KeyID())
	return nil
}

// ── migrate ─────────────────────────────────────────────────

func runMigrate(args []string) error {
	if len(args) == 0 {
		return errors.New("migrate: expected up|down|status|version|create")
	}

	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}

	action := args[0]

	if action == "create" {
		if len(args) < 2 {
			return errors.New("migrate create: expected a migration name")
		}
		// db is unused when a migration type is supplied, so nil is safe here.
		return goose.Create(nil, "db/migrations", args[1], "sql")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := logging.New(cfg)

	db, err := openMigrateDB(cfg)
	if err != nil {
		return err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}

	switch action {
	case "up":
		if err := goose.UpContext(ctx, db, "."); err != nil {
			return err
		}
		v, _ := goose.GetDBVersionContext(ctx, db)
		logger.Info("migrations applied", slog.Int64("schema_version", v))
	case "down":
		if err := goose.DownContext(ctx, db, "."); err != nil {
			return err
		}
	case "status":
		if err := goose.StatusContext(ctx, db, "."); err != nil {
			return err
		}
	case "version":
		v, err := goose.GetDBVersionContext(ctx, db)
		if err != nil {
			return err
		}
		fmt.Printf("schema version: %d\n", v)
	default:
		return fmt.Errorf("migrate: unknown action %q", action)
	}
	return nil
}

func openMigrateDB(cfg *config.Config) (*sql.DB, error) {
	dsn := cfg.MigrateURL
	if dsn == "" {
		dsn = cfg.DatabaseURL
	}
	if dsn == "" {
		return nil, errors.New("set ATLAS_MIGRATE_DATABASE_URL (preferred) or ATLAS_DATABASE_URL")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	return db, nil
}

// ── outbox ──────────────────────────────────────────────────

func runOutbox(args []string) error {
	if len(args) == 0 || args[0] != "hello" {
		return errors.New("outbox: expected `hello [message]`")
	}

	fs := flag.NewFlagSet("outbox hello", flag.ContinueOnError)
	message := fs.String("message", "", "message to include in the hello event")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *message == "" && fs.NArg() > 0 {
		*message = fs.Arg(0)
	}
	if *message == "" {
		*message = "hello from atlas-cli"
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := logging.New(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := database.Connect(ctx, cfg.DatabaseURL, logger)
	if err != nil {
		return err
	}
	defer pool.Close()

	event, err := outbox.NewHello(nil, nil, *message)
	if err != nil {
		return err
	}

	seq, err := outbox.Write(ctx, pool, event)
	if err != nil {
		return err
	}

	out, _ := json.MarshalIndent(map[string]any{
		"event_id": event.ID.String(),
		"type":     event.Type,
		"seq":      seq,
	}, "", "  ")
	fmt.Println(string(out))
	return nil
}

// ── seed ────────────────────────────────────────────────────

func runSeed() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := logging.New(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	pool, err := database.Connect(ctx, cfg.DatabaseURL, logger)
	if err != nil {
		return err
	}
	defer pool.Close()

	report, err := seed.Run(ctx, pool)
	if err != nil {
		return err
	}

	logger.Info("seed complete",
		slog.Int("roles", report.Roles),
		slog.Int("permissions", report.Permissions),
		slog.Int("role_permissions", report.RolePermissions),
		slog.Int("settings_added", report.SettingsAdded),
	)
	return nil
}

// ── bootstrap ───────────────────────────────────────────────

// runBootstrap creates the first account. Atlas has no self-service sign-up, so
// someone with operator access has to create the first person deliberately; every
// account after that comes from an invitation. The password is never a default —
// pass --password or set ATLAS_BOOTSTRAP_PASSWORD.
func runBootstrap(args []string) error {
	fs := flag.NewFlagSet("bootstrap", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `atlas-cli bootstrap — create the first Atlas account

  --email <address>     required; also the sign-in identifier
  --name <full name>    required; the real person's name
  --role <role key>     default system_admin (see §3.2)
  --phone <E.164>       optional second sign-in identifier
  --scope <scope>       default company; one of company|business_unit|department|project|own
  --password <secret>   at least 12 characters, or set ATLAS_BOOTSTRAP_PASSWORD
`)
	}
	email := fs.String("email", "", "email address (also the sign-in identifier)")
	name := fs.String("name", "", "the person's full name")
	role := fs.String("role", "system_admin", "role key from §3.2")
	phone := fs.String("phone", "", "optional E.164 phone number")
	scope := fs.String("scope", "company", "grant scope type")
	password := fs.String("password", os.Getenv("ATLAS_BOOTSTRAP_PASSWORD"), "password (min 12); prefer the env var")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *email == "" || *name == "" {
		fs.Usage()
		return errors.New("bootstrap: --email and --name are required")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := logging.New(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	pool, err := database.Connect(ctx, cfg.DatabaseURL, logger)
	if err != nil {
		return err
	}
	defer pool.Close()

	account, err := auth.Bootstrap(ctx, pool, auth.BootstrapInput{
		Email:     *email,
		FullName:  *name,
		Phone:     *phone,
		Password:  *password,
		RoleKey:   *role,
		ScopeType: *scope,
	})
	if err != nil {
		return err
	}

	logger.Info("first account created",
		slog.String("email", account.Email),
		slog.String("account_id", account.AccountID.String()),
		slog.String("person_id", account.PersonID.String()),
		slog.String("role", *role),
	)
	fmt.Fprintln(os.Stdout, "Sign in with your email (or the phone number) and the password you supplied.")
	if auth.RoleRequiresMFA(*role) {
		fmt.Fprintln(os.Stdout, "This role is privileged: the first sign-in is a TOTP enrolment.")
	}
	return nil
}
