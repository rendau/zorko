package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/rendau/zorko/internal/config"
	"github.com/rendau/zorko/internal/constant"
	domainOrgRepoDbP "github.com/rendau/zorko/internal/domain/org/repo/db"
	domainOrgServiceP "github.com/rendau/zorko/internal/domain/org/service"
	domainProjectRepoDbP "github.com/rendau/zorko/internal/domain/project/repo/db"
	domainProjectServiceP "github.com/rendau/zorko/internal/domain/project/service"
	domainProjectKeyRepoDbP "github.com/rendau/zorko/internal/domain/project_key/repo/db"
	domainProjectKeyServiceP "github.com/rendau/zorko/internal/domain/project_key/service"
	handlerGrpcP "github.com/rendau/zorko/internal/handler/grpc"
	usecaseOrgP "github.com/rendau/zorko/internal/usecase/org"
	usecaseProjectP "github.com/rendau/zorko/internal/usecase/project"
	usecaseProjectKeyP "github.com/rendau/zorko/internal/usecase/project_key"
	"github.com/rendau/zorko/pkg/proto/zorko_v1"
)

type App struct {
	globalTracerCloser io.Closer

	pgpool *pgxpool.Pool
	chConn driver.Conn

	grpcServer       *GrpcServer
	httpServer       *http.Server
	systemHttpServer *http.Server

	ctx       context.Context
	ctxCancel context.CancelFunc

	exitCode int
}

func (a *App) Init() {
	var err error

	a.ctx, a.ctxCancel = context.WithCancel(context.Background())

	// logger
	initLogger(config.Conf.Debug, config.Conf.LogLevel)

	// globalTracer
	{
		if config.Conf.WithTracing && config.Conf.JaegerAddress != "" {
			slog.Info("tracing enabled")
			_, a.globalTracerCloser, err = tracerInitGlobal(config.Conf.JaegerAddress, constant.ServiceName)
			errCheck(err, "tracerInitGlobal")
		}
	}

	// pgpool
	a.pgpool, err = initPgPool(config.Conf.PgDsn)
	errCheck(err, "pgpool init")

	// migrations
	{
		runMigrations()
		slog.Info("PG-migrations have been successfully applied")
	}

	// clickhouse
	a.chConn, err = initChConn(config.Conf.ChDsn)
	errCheck(err, "clickhouse init")

	// ch-migrations
	{
		runChMigrations(a.ctx, a.chConn)
		slog.Info("CH-migrations have been successfully applied")
	}

	// org
	orgRepo := domainOrgRepoDbP.New(a.pgpool)
	orgService := domainOrgServiceP.New(orgRepo)
	orgUsecase := usecaseOrgP.New(orgService)
	orgHandler := handlerGrpcP.NewOrg(orgUsecase)

	// project
	projectRepo := domainProjectRepoDbP.New(a.pgpool)
	projectService := domainProjectServiceP.New(projectRepo)
	projectUsecase := usecaseProjectP.New(projectService)
	projectHandler := handlerGrpcP.NewProject(projectUsecase)

	// project_key
	projectKeyRepo := domainProjectKeyRepoDbP.New(a.pgpool)
	projectKeyService := domainProjectKeyServiceP.New(projectKeyRepo)
	projectKeyUsecase := usecaseProjectKeyP.New(projectKeyService)
	projectKeyHandler := handlerGrpcP.NewProjectKey(projectKeyUsecase)

	// grpc server
	{
		a.grpcServer = NewGrpcServer("main", func(server *grpc.Server) {
			zorko_v1.RegisterOrgServiceServer(server, orgHandler)
			zorko_v1.RegisterProjectServiceServer(server, projectHandler)
			zorko_v1.RegisterProjectKeyServiceServer(server, projectKeyHandler)
		})
	}

	// http-gw server
	{
		var handler http.Handler

		handler, err = GrpcGatewayCreateHandler(func(mux *runtime.ServeMux) error {
			opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}

			var conn *grpc.ClientConn
			conn, err = grpc.NewClient("localhost:"+config.Conf.GrpcPort, opts...)
			errCheck(err, "grpc.Dial")

			// register grpc handlers
			handlers := []func(context.Context, *runtime.ServeMux, *grpc.ClientConn) error{
				zorko_v1.RegisterOrgServiceHandler,
				zorko_v1.RegisterProjectServiceHandler,
				zorko_v1.RegisterProjectKeyServiceHandler,
			}
			for _, h := range handlers {
				err = h(context.Background(), mux, conn)
				if err != nil {
					return fmt.Errorf("grpc-gateway: register grpc-handler: %w", err)
				}
			}

			// custom http handlers
			httpHandlers := []struct {
				method  string
				path    string
				handler runtime.HandlerFunc
			}{}
			for _, h := range httpHandlers {
				err = mux.HandlePath(h.method, h.path, h.handler)
				if err != nil {
					return fmt.Errorf("grpc-gateway: register http-handler: %w", err)
				}
			}

			return nil
		})
		errCheck(err, "grpcGatewayCreateHandler")

		// server
		a.httpServer = &http.Server{
			Addr:              ":" + config.Conf.HttpPort,
			Handler:           handler,
			ReadHeaderTimeout: 2 * time.Second,
			ReadTimeout:       time.Minute,
			MaxHeaderBytes:    300 * 1024,
		}
	}

	// system http server (healthcheck, docs, metrics)
	{
		a.systemHttpServer = SystemHttpServerCreate()
	}
}

func (a *App) PreStartHook() {
	slog.Info("PreStartHook")
}

func (a *App) Start() {
	slog.Info("Starting", "version", constant.Version, "commit", constant.Commit, "built_at", constant.BuiltAt)

	// grpc server
	{
		err := a.grpcServer.Start()
		errCheck(err, "grpcServer.Start")
	}

	// http-gw server
	{
		go func() {
			err := a.httpServer.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				// errCheck(err, "http-server stopped")
			}
		}()
		slog.Info("http-server started " + a.httpServer.Addr)
	}

	// system http server
	{
		go func() {
			err := a.systemHttpServer.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				// errCheck(err, "system-http-server stopped")
			}
		}()
		slog.Info("system-http-server started " + a.systemHttpServer.Addr)
	}
}

func (a *App) Listen() {
	signalCtx, signalCtxCancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer signalCtxCancel()

	// wait signal
	<-signalCtx.Done()
}

func (a *App) Stop() {
	slog.Info("Shutting down...")

	// stop context
	a.ctxCancel()

	// http-gw server
	{
		ctx, ctxCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer ctxCancel()

		if err := a.httpServer.Shutdown(ctx); err != nil {
			slog.Error("http-server shutdown error", "error", err)
			a.exitCode = 1
		}
	}

	// system http server
	{
		ctx, ctxCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer ctxCancel()

		if err := a.systemHttpServer.Shutdown(ctx); err != nil {
			slog.Error("system-http-server shutdown error", "error", err)
			a.exitCode = 1
		}
	}

	// grpc server
	a.grpcServer.Stop()
}

func (a *App) WaitJobs() {
	slog.Info("waiting jobs")
}

func (a *App) Exit() {
	slog.Info("Exit")

	if a.globalTracerCloser != nil {
		_ = a.globalTracerCloser.Close()
	}

	_ = a.chConn.Close()

	a.pgpool.Close()

	// flush stdout

	os.Exit(a.exitCode)
}

func errCheck(err error, msg string) {
	if err != nil {
		if msg != "" {
			err = fmt.Errorf("%s: %w", msg, err)
		}
		slog.Error(err.Error())
		os.Exit(1)
	}
}
