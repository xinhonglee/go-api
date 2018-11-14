package server

import (
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/render"
	config "github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/snowzach/gorestapi/gorestapi"
)

type Server struct {
	logger     *zap.SugaredLogger
	router     chi.Router
	thingStore gorestapi.ThingStore
}

// New will setup the API listener
func New(thingStore gorestapi.ThingStore) (*Server, error) {

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(middleware.URLFormat)
	r.Use(render.SetContentType(render.ContentTypeJSON))

	// Log Requests
	if config.GetBool("server.log_requests") {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				start := time.Now()
				var requestID string
				if reqID := r.Context().Value(middleware.RequestIDKey); reqID != nil {
					requestID = reqID.(string)
				}
				ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
				next.ServeHTTP(ww, r)

				latency := time.Since(start)

				fields := []zapcore.Field{
					zap.Int("status", ww.Status()),
					zap.Duration("took", latency),
					zap.String("remote", r.RemoteAddr),
					zap.String("request", r.RequestURI),
					zap.String("method", r.Method),
					zap.String("package", "server.request"),
				}
				if requestID != "" {
					fields = append(fields, zap.String("request-id", requestID))
				}
				zap.L().Info("API Request", fields...)
			})
		})
	}

	// Enable profiler
	if config.GetBool("server.profiler_enabled") && config.GetString("server.profiler_path") != "" {
		zap.S().Debugw("Profiler enabled on API", "path", config.GetString("server.profiler_path"))
		r.Mount(config.GetString("server.profiler_path"), middleware.Profiler())
	}

	s := &Server{
		logger:     zap.S().With("package", "api"),
		router:     r,
		thingStore: thingStore,
	}

	address := net.JoinHostPort(config.GetString("server.host"), config.GetString("server.port"))
	go func() {
		if err := http.ListenAndServe(address, s.router); err != nil {
			s.logger.Fatalw("API Listen error", "error", err, "address", address)
		}
	}()
	s.logger.Infow("API Listening", "address", address)

	s.SetupRoutes()

	return s, nil

}

// RenderOrErrInternal will render whatever you pass it (assuming it has Renderer) or prints an internal error
func RenderOrErrInternal(w http.ResponseWriter, r *http.Request, d render.Renderer) {
	if err := render.Render(w, r, d); err != nil {
		render.Render(w, r, ErrInternal(err))
		return
	}
}
