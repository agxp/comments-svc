package main

import (
	pb "github.com/agxp/cloudflix/comments-svc/proto"
	"github.com/micro/go-micro"
	"time"
	_ "github.com/micro/go-plugins/registry/kubernetes"
	jaegercfg "github.com/uber/jaeger-client-go/config"
	micro_opentracing "github.com/micro/go-plugins/wrapper/trace/opentracing"
	"github.com/opentracing/opentracing-go"
	"github.com/uber/jaeger-lib/metrics/prometheus"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"
	zapWrapper "github.com/uber/jaeger-client-go/log/zap"
)

var (
	tracer *opentracing.Tracer
	logger         *zap.Logger
	metricsFactory metrics.Factory
)


func main() {

	logger, _ = zap.NewDevelopment()
	metricsFactory = prometheus.New()

	zapLogger := logger.With(zap.String("service", "comments-svc"))
	jeagerLogger := zapWrapper.NewLogger(zapLogger)

	cfg, err := jaegercfg.FromEnv()
	if err != nil {
		// parsing errors might happen here, such as when we get a string where we expect a number
		zapLogger.Error("Could not parse Jaeger env vars: %s", zap.Error(err))
		return
	}

	t, closer, err := cfg.NewTracer(
		jaegercfg.Metrics(metricsFactory),
		jaegercfg.Logger(jeagerLogger),
	)
	if err != nil {
		jeagerLogger.Infof("Could not initialize jaeger tracer: %s", err)
		return
	}

	tracer = &t
	opentracing.SetGlobalTracer(t)
	defer closer.Close()

	(*tracer).StartSpan("init_tracing").Finish()
	// continue main()

	// Creates a database connection and handles
	// closing it again before exit.
	s3, err := ConnectToS3()
	if err != nil {
		jeagerLogger.Error("Could not connect to store: " + err.Error())
	}

	pg, err := ConnectToPostgres()
	if err != nil {
		jeagerLogger.Error("Could not connect to database: " + err.Error())
	}

	cache, err := ConnectToRedis()
	if err != nil {
		jeagerLogger.Error("Could not connect to cache: " + err.Error())
	}

	repo := &CommentRepository{s3, pg, cache,tracer}

	// Create a new service. Optionally include some options here.
	srv := micro.NewService(

		// This name must match the package name given in your protobuf definition
		micro.Name("comments"),
		micro.Version("latest"),
		micro.RegisterTTL(time.Second*30),
		micro.RegisterInterval(time.Second*10),
		micro.WrapHandler(micro_opentracing.NewHandlerWrapper(*tracer)),
	)

	// Init will parse the command line flags.
	srv.Init()

	// Will comment this out now to save having to run this locally
	// publisher := micro.NewPublisher("user.created", srv.Client())

	// Register handler
	pb.RegisterCommentsHandler(srv.Server(), &service{repo, tracer, zapLogger})

	// Run the server
	if err := srv.Run(); err != nil {
		jeagerLogger.Error(err.Error())
	}
}