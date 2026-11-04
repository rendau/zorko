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
	"runtime/debug"
	"syscall"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/rendau/zorko/internal/config"
	"github.com/rendau/zorko/internal/constant"
	domainArtifactRepoBlobP "github.com/rendau/zorko/internal/domain/artifact/repo/blob"
	domainArtifactRepoDbP "github.com/rendau/zorko/internal/domain/artifact/repo/db"
	domainArtifactServiceP "github.com/rendau/zorko/internal/domain/artifact/service"
	domainEventRepoChP "github.com/rendau/zorko/internal/domain/event/repo/ch"
	domainEventServiceP "github.com/rendau/zorko/internal/domain/event/service"
	domainIssueRepoDbP "github.com/rendau/zorko/internal/domain/issue/repo/db"
	domainIssueServiceP "github.com/rendau/zorko/internal/domain/issue/service"
	domainOrgRepoDbP "github.com/rendau/zorko/internal/domain/org/repo/db"
	domainOrgServiceP "github.com/rendau/zorko/internal/domain/org/service"
	domainOrgTokenRepoDbP "github.com/rendau/zorko/internal/domain/org_token/repo/db"
	domainOrgTokenServiceP "github.com/rendau/zorko/internal/domain/org_token/service"
	domainProjectRepoDbP "github.com/rendau/zorko/internal/domain/project/repo/db"
	domainProjectServiceP "github.com/rendau/zorko/internal/domain/project/service"
	domainProjectKeyRepoDbP "github.com/rendau/zorko/internal/domain/project_key/repo/db"
	domainProjectKeyServiceP "github.com/rendau/zorko/internal/domain/project_key/service"
	domainReleaseRepoDbP "github.com/rendau/zorko/internal/domain/release/repo/db"
	domainReleaseServiceP "github.com/rendau/zorko/internal/domain/release/service"
	domainSessionServiceP "github.com/rendau/zorko/internal/domain/session/service"
	domainUsrRepoDbP "github.com/rendau/zorko/internal/domain/usr/repo/db"
	domainUsrServiceP "github.com/rendau/zorko/internal/domain/usr/service"
	handlerGrpcP "github.com/rendau/zorko/internal/handler/grpc"
	handlerIngestP "github.com/rendau/zorko/internal/handler/ingest"
	handlerSentryapiP "github.com/rendau/zorko/internal/handler/sentryapi"
	"github.com/rendau/zorko/internal/infra/chmigrate"
	infraS3P "github.com/rendau/zorko/internal/infra/s3"
	serviceAssembleServiceP "github.com/rendau/zorko/internal/service/assemble/service"
	serviceGroupingServiceP "github.com/rendau/zorko/internal/service/grouping/service"
	serviceKeycacheServiceP "github.com/rendau/zorko/internal/service/keycache/service"
	serviceLoginlimitServiceP "github.com/rendau/zorko/internal/service/loginlimit/service"
	serviceNormalizeServiceP "github.com/rendau/zorko/internal/service/normalize/service"
	serviceProcessorServiceP "github.com/rendau/zorko/internal/service/processor/service"
	serviceQueueServiceP "github.com/rendau/zorko/internal/service/queue/service"
	serviceRatelimitServiceP "github.com/rendau/zorko/internal/service/ratelimit/service"
	serviceSearchqueryServiceP "github.com/rendau/zorko/internal/service/searchquery/service"
	usecaseIngestP "github.com/rendau/zorko/internal/usecase/ingest"
	usecaseIssueP "github.com/rendau/zorko/internal/usecase/issue"
	usecaseOrgP "github.com/rendau/zorko/internal/usecase/org"
	usecaseOrgTokenP "github.com/rendau/zorko/internal/usecase/org_token"
	usecaseProjectP "github.com/rendau/zorko/internal/usecase/project"
	usecaseProjectKeyP "github.com/rendau/zorko/internal/usecase/project_key"
	usecaseUploadP "github.com/rendau/zorko/internal/usecase/upload"
	usecaseUsrP "github.com/rendau/zorko/internal/usecase/usr"
	"github.com/rendau/zorko/pkg/proto/zorko_v1"
)

type App struct {
	globalTracerCloser io.Closer

	pgpool *pgxpool.Pool
	chConn driver.Conn
	queue  *serviceQueueServiceP.Service

	keyCache  *serviceKeycacheServiceP.Service
	assembler *serviceAssembleServiceP.Service
	processor *serviceProcessorServiceP.Service

	grpcServer       *GrpcServer
	httpServer       *http.Server
	ingestHttpServer *http.Server
	adminHttpServer  *http.Server
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
		chmigrate.Run(a.ctx, a.chConn, "./migrations-ch")
		slog.Info("CH-migrations have been successfully applied")
	}

	// s3 + artifact (карты исходников, ADR-009); без S3_ENDPOINT — выключены
	var artifactService *domainArtifactServiceP.Service
	if config.Conf.S3Endpoint != "" {
		s3Client, err := infraS3P.New(infraS3P.Config{
			Endpoint:  config.Conf.S3Endpoint,
			Region:    config.Conf.S3Region,
			Bucket:    config.Conf.S3Bucket,
			AccessKey: config.Conf.S3AccessKey,
			SecretKey: config.Conf.S3SecretKey,
			Secure:    config.Conf.S3Secure,
		})
		errCheck(err, "s3 init")
		ctx, ctxCancel := context.WithTimeout(a.ctx, 15*time.Second)
		errCheck(s3Client.EnsureBucket(ctx), "s3 EnsureBucket")
		ctxCancel()
		artifactService = domainArtifactServiceP.New(domainArtifactRepoDbP.New(a.pgpool), domainArtifactRepoBlobP.New(s3Client))
	} else {
		slog.Warn("S3_ENDPOINT is empty: source maps are disabled")
	}

	// session
	if config.Conf.JwtSecret == "" {
		errCheck(errors.New("JWT_SECRET is empty"), "session init")
	}
	sessionService := domainSessionServiceP.New(config.Conf.JwtSecret)

	// usr
	usrRepo := domainUsrRepoDbP.New(a.pgpool)
	usrService := domainUsrServiceP.New(usrRepo)
	usrUsecase := usecaseUsrP.New(usrService, sessionService, serviceLoginlimitServiceP.New(5, time.Minute))
	usrHandler := handlerGrpcP.NewUsr(usrUsecase)

	// org
	orgRepo := domainOrgRepoDbP.New(a.pgpool)
	orgService := domainOrgServiceP.New(orgRepo)
	orgUsecase := usecaseOrgP.New(orgService, sessionService)
	orgHandler := handlerGrpcP.NewOrg(orgUsecase)

	// project
	projectRepo := domainProjectRepoDbP.New(a.pgpool)
	projectService := domainProjectServiceP.New(projectRepo)
	projectUsecase := usecaseProjectP.New(projectService, sessionService)
	projectHandler := handlerGrpcP.NewProject(projectUsecase)

	// project_key
	projectKeyRepo := domainProjectKeyRepoDbP.New(a.pgpool)
	projectKeyService := domainProjectKeyServiceP.New(projectKeyRepo)
	projectKeyUsecase := usecaseProjectKeyP.New(projectKeyService, sessionService)
	projectKeyHandler := handlerGrpcP.NewProjectKey(projectKeyUsecase)

	// org_token (токены CI, ADR-009)
	orgTokenRepo := domainOrgTokenRepoDbP.New(a.pgpool)
	orgTokenService := domainOrgTokenServiceP.New(orgTokenRepo)
	orgTokenUsecase := usecaseOrgTokenP.New(orgTokenService, orgService, sessionService)
	orgTokenHandler := handlerGrpcP.NewOrgToken(orgTokenUsecase)

	// queue
	{
		if len(config.Conf.KafkaBrokers) == 0 && (config.Conf.IngestEnabled || config.Conf.ProcessorEnabled) {
			errCheck(errors.New("KAFKA_BROKERS is empty"), "queue init")
		}

		a.queue, err = serviceQueueServiceP.New(serviceQueueServiceP.Config{
			Brokers:         config.Conf.KafkaBrokers,
			Topic:           config.Conf.KafkaTopicEvents,
			ConsumerGroup:   config.Conf.KafkaConsumerGroup,
			TopicPartitions: config.Conf.KafkaTopicPartitions,
			TopicRetention:  config.Conf.KafkaTopicRetention,
			WithProducer:    config.Conf.IngestEnabled,
			WithConsumer:    config.Conf.ProcessorEnabled,
			FetchMaxWait:    config.Conf.ProcessorBatchWait,
		})
		errCheck(err, "queue init")

		ctx, ctxCancel := context.WithTimeout(a.ctx, 15*time.Second)
		err = a.queue.EnsureTopic(ctx)
		ctxCancel()
		errCheck(err, "queue.EnsureTopic")
	}

	// ingest
	if config.Conf.IngestEnabled {
		a.keyCache = serviceKeycacheServiceP.New(projectKeyService, 30*time.Second)
		rateLimiter := serviceRatelimitServiceP.New()
		ingestUsecase := usecaseIngestP.New(a.keyCache, rateLimiter, a.queue)
		ingestHandler := handlerIngestP.New(ingestUsecase, config.Conf.IngestMaxBodyBytes, config.Conf.IngestMaxEventBytes)

		// Sentry API для sentry-cli: заливка карт и релизы (на том же адресе, что DSN)
		if artifactService != nil {
			a.assembler = serviceAssembleServiceP.New(serviceAssembleServiceP.Config{TmpDir: config.Conf.AssembleTmpDir}, artifactService)
		}
		releaseService := domainReleaseServiceP.New(domainReleaseRepoDbP.New(a.pgpool))
		uploadUsecase := usecaseUploadP.New(artifactService != nil, orgTokenService, orgService, projectService,
			artifactService, a.assembler, releaseService)
		sentryapiHandler := handlerSentryapiP.New(uploadUsecase)

		ingestMux := http.NewServeMux()
		ingestMux.Handle("/api/0/", sentryapiHandler.Router())
		ingestMux.Handle("/", ingestHandler.Router())

		a.ingestHttpServer = &http.Server{
			Addr:              ":" + config.Conf.IngestPort,
			Handler:           recoverMiddleware(ingestMux),
			ReadHeaderTimeout: 2 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       2 * time.Minute,
			MaxHeaderBytes:    64 * 1024,
		}
	}

	// processor
	if config.Conf.ProcessorEnabled {
		issueRepo := domainIssueRepoDbP.New(a.pgpool)
		issueService := domainIssueServiceP.New(issueRepo)

		eventRepo := domainEventRepoChP.New(a.chConn)
		eventService := domainEventServiceP.New(eventRepo)

		a.processor = serviceProcessorServiceP.New(serviceProcessorServiceP.Config{
			BatchSize:      config.Conf.ProcessorBatchSize,
			StorePerMinute: int64(config.Conf.IssueEventStorePerMinute),
			RetryMin:       time.Second,
			RetryMax:       30 * time.Second,
			StageTimeout:   30 * time.Second,
		}, a.queue, serviceNormalizeServiceP.New(), serviceGroupingServiceP.New(), issueService, eventService)
	}

	// issue, event, tag (чтение для интерфейса)
	issueReadService := domainIssueServiceP.New(domainIssueRepoDbP.New(a.pgpool))
	eventReadService := domainEventServiceP.NewRead(domainEventRepoChP.New(a.chConn))
	issueUsecase := usecaseIssueP.New(sessionService, projectService, issueReadService, eventReadService, serviceSearchqueryServiceP.New())
	issueHandler := handlerGrpcP.NewIssue(issueUsecase)
	eventHandler := handlerGrpcP.NewEvent(issueUsecase)
	tagHandler := handlerGrpcP.NewTag(issueUsecase)

	// grpc server
	{
		a.grpcServer = NewGrpcServer("main", sessionService, func(server *grpc.Server) {
			zorko_v1.RegisterUsrServiceServer(server, usrHandler)
			zorko_v1.RegisterOrgServiceServer(server, orgHandler)
			zorko_v1.RegisterProjectServiceServer(server, projectHandler)
			zorko_v1.RegisterProjectKeyServiceServer(server, projectKeyHandler)
			zorko_v1.RegisterOrgTokenServiceServer(server, orgTokenHandler)
			zorko_v1.RegisterIssueServiceServer(server, issueHandler)
			zorko_v1.RegisterEventServiceServer(server, eventHandler)
			zorko_v1.RegisterTagServiceServer(server, tagHandler)
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
				zorko_v1.RegisterUsrServiceHandler,
				zorko_v1.RegisterOrgServiceHandler,
				zorko_v1.RegisterProjectServiceHandler,
				zorko_v1.RegisterProjectKeyServiceHandler,
				zorko_v1.RegisterOrgTokenServiceHandler,
				zorko_v1.RegisterIssueServiceHandler,
				zorko_v1.RegisterEventServiceHandler,
				zorko_v1.RegisterTagServiceHandler,
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

	// admin http server (интерфейс)
	{
		a.adminHttpServer = AdminHttpServerCreate(config.Conf.AdminPort, config.Conf.AdminDir, config.Conf.AdminApiBaseUrl)
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

	// key cache
	if a.keyCache != nil {
		a.keyCache.Start(a.ctx)
	}

	// assembler (сборка архивов карт)
	if a.assembler != nil {
		a.assembler.Start(a.ctx)
	}

	// processor
	if a.processor != nil {
		a.processor.Start(a.ctx)
		slog.Info("processor started")
	}

	// ingest http server
	if a.ingestHttpServer != nil {
		go func() {
			err := a.ingestHttpServer.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCheck(err, "ingest-http-server stopped")
			}
		}()
		slog.Info("ingest-http-server started " + a.ingestHttpServer.Addr)
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

	// admin http server
	if a.adminHttpServer != nil {
		go func() {
			err := a.adminHttpServer.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCheck(err, "admin-http-server stopped")
			}
		}()
		slog.Info("admin-http-server started " + a.adminHttpServer.Addr)
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

	// ingest http server: новые события больше не принимаются, начатые запросы дописываются в очередь
	if a.ingestHttpServer != nil {
		ctx, ctxCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer ctxCancel()

		if err := a.ingestHttpServer.Shutdown(ctx); err != nil {
			slog.Error("ingest-http-server shutdown error", "error", err)
			a.exitCode = 1
		}
	}

	// http-gw server
	{
		ctx, ctxCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer ctxCancel()

		if err := a.httpServer.Shutdown(ctx); err != nil {
			slog.Error("http-server shutdown error", "error", err)
			a.exitCode = 1
		}
	}

	// admin http server
	if a.adminHttpServer != nil {
		ctx, ctxCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer ctxCancel()

		if err := a.adminHttpServer.Shutdown(ctx); err != nil {
			slog.Error("admin-http-server shutdown error", "error", err)
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

	// key cache
	if a.keyCache != nil {
		a.keyCache.Wait()
	}

	// assembler: текущие сборки отменены контекстом и записывают своё состояние
	if a.assembler != nil {
		a.assembler.Wait()
	}

	// processor: дописывает и подтверждает текущую пачку
	if a.processor != nil {
		a.processor.Wait()
	}
}

func (a *App) Exit() {
	slog.Info("Exit")

	if a.globalTracerCloser != nil {
		_ = a.globalTracerCloser.Close()
	}

	a.queue.Close()

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

// recoverMiddleware — паника в обработчике запроса не роняет сервис
func recoverMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error(
					"Recovered from panic",
					slog.Any("error", err),
					slog.Any("recovery_stacktrace", string(debug.Stack())),
				)
				w.WriteHeader(http.StatusInternalServerError)
			}
		}()
		h.ServeHTTP(w, r)
	})
}
