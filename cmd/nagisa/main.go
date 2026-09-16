package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"nagisa/internal/conf"
	"nagisa/internal/data"

	"github.com/go-kratos/kratos/contrib/otel/v3/tracing"
	"github.com/go-kratos/kratos/v3"
	"github.com/go-kratos/kratos/v3/config"
	"github.com/go-kratos/kratos/v3/config/env"
	"github.com/go-kratos/kratos/v3/config/file"
	"github.com/go-kratos/kratos/v3/log"
	"github.com/go-kratos/kratos/v3/transport/grpc"
	"github.com/go-kratos/kratos/v3/transport/http"

	_ "go.uber.org/automaxprocs"
)

// The Windows shell icon is linked from a resource object built out of
// icon.ico. The object is committed, so an ordinary build already carries the
// icon; rerun this only after replacing icon.ico.
//
//go:generate go run github.com/akavel/rsrc@v0.10.2 -ico icon.ico -o rsrc_windows_amd64.syso -arch amd64

// go build -ldflags "-X main.Version=x.y.z"
var (
	// Name is the name of the compiled software.
	Name = "nagisa-netdisk"
	// Version is the version of the compiled software.
	Version = "dev"
	// flagconf is the config flag.
	flagconf string

	id, _ = os.Hostname()
)

func init() {
	flag.StringVar(&flagconf, "conf", "../../configs", "config path, eg: -conf config.yaml")
}

func newApp(logger *slog.Logger, gs *grpc.Server, hs *http.Server, _ *data.Seeder) *kratos.App {
	return kratos.New(
		kratos.ID(id),
		kratos.Name(Name),
		kratos.Version(Version),
		kratos.Metadata(map[string]string{}),
		kratos.Logger(logger),
		kratos.Server(
			gs,
			hs,
		),
	)
}

// configSource resolves the -conf flag into a single file. A directory is read
// as <dir>/config.yaml rather than as "every file in the directory", because
// the kratos file source tries to decode each of them and fails on any
// extension it does not recognise. Dropping a log, a database or a README next
// to config.yaml would otherwise take the process down at startup.
func configSource(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("config path %q: %w", path, err)
	}
	if !info.IsDir() {
		return path, nil
	}
	candidate := filepath.Join(path, "config.yaml")
	if _, err := os.Stat(candidate); err != nil {
		return "", fmt.Errorf("config directory %q has no config.yaml", path)
	}
	return candidate, nil
}

func main() {
	flag.Parse()
	logger := log.NewLogger(
		slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelInfo,
		}),
		log.WithExtractor(tracing.TraceAttrs),
	).With(
		slog.String("service.id", id),
		slog.String("service.name", Name),
		slog.String("service.version", Version),
	)
	log.SetDefault(logger)

	source, err := configSource(flagconf)
	if err != nil {
		panic(err)
	}
	c := config.New(
		config.WithSource(
			file.NewSource(source),
			// Values from the environment win over the file. The key is the
			// dotted config path, for example `KRATOS_server.http.addr` or
			// `KRATOS_auth.jwt_secret`.
			env.NewSource("KRATOS"),
		),
	)
	defer c.Close()

	if err := c.Load(); err != nil {
		panic(err)
	}

	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		panic(err)
	}

	app, cleanup, err := wireApp(&bc, logger)
	if err != nil {
		panic(err)
	}
	defer cleanup()

	// start and wait for stop signal
	if err := app.Run(); err != nil {
		panic(err)
	}
}
