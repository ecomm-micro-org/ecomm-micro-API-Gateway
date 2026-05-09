package routes

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"github.com/gorilla/mux"
	"github.com/hudl/fargo"
	"github.com/risbern21/api_gateway/handler"
	"github.com/risbern21/api_gateway/internal/logger"
	"github.com/risbern21/api_gateway/internal/middleware"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

const (
	GET    = "GET"
	POST   = "POST"
	PUT    = "PUT"
	DELETE = "DELETE"
)

func AddRoutes(r *mux.Router, serviceRegistry string) {
	secretKey := os.Getenv("SECRET_KEY")
	if secretKey == "" {
		log.Fatal("no scret key defined")
	}

	h := handler.NewHandler(secretKey)
	m := middleware.NewMiddleware(secretKey)

	r.Use(m.LoggingMiddleware)

	//health endpoint
	r.HandleFunc("/api/health", h.Health)

	//api doc endpoint
	r.PathPrefix("/swagger/").Handler(httpSwagger.Handler(
		httpSwagger.URL("http://localhost:6969/swagger/doc.json"), //The url pointing to API definition
		httpSwagger.DeepLinking(true),
		httpSwagger.DocExpansion("none"),
		httpSwagger.DomID("swagger-ui"),
	)).Methods(http.MethodGet)

	r.HandleFunc("/api/auth/signin", h.CreateUser).Methods(POST)
	r.HandleFunc("/api/auth/login", h.Login).Methods(POST)
	r.HandleFunc("/api/auth/logout", h.Logout).Methods(POST)
	r.HandleFunc("/api/tokens/renew", h.RenewAccessToken).Methods(POST)
	r.HandleFunc("/api/tokens/revoke", h.RevokeSession).Methods(POST)

	addProductRoutes(r, m, serviceRegistry)
	addOrderRoutes(r, m, serviceRegistry)
	addChatRoutes(r, m, serviceRegistry)
	addGenerationRoutes(r, m, serviceRegistry)
}

func newProxy(serviceURL string) http.HandlerFunc {
	target, err := url.Parse(serviceURL)
	if err != nil {
		log.Fatalf("unable to parse target url : %v", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logger.Log().Infof("Proxy error for request %s %s: %v", r.Method, r.URL.Path, err)
		http.Error(w, "Service unavailable", http.StatusBadGateway)
	}

	return func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
		r.URL.Host = target.Host

		logger.Log().Infof("Proxying request to %s%s", target.String(), r.URL.Path)
		proxy.ServeHTTP(w, r)
	}
}

func getServiceURL(conn fargo.EurekaConnection, serviceName string) (string, error) {
	app, err := conn.GetApp(serviceName)
	if err != nil || len(app.Instances) == 0 {
		return "", fmt.Errorf("no instances for %s", serviceName)
	}

	instance := app.Instances[rand.Intn(len(app.Instances))]
	return fmt.Sprintf("http://%s:%d", instance.HostName, instance.Port), nil
}
