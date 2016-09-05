package rest

import (
	"context"
	"github.com/hashicorp/consul/api"
	"github.com/julienschmidt/httprouter"
	"github.com/justinas/alice"
	"net"
	"net/http"
	"novaforge.bull.com/starlings-janus/janus/log"
	"novaforge.bull.com/starlings-janus/janus/tasks"
)

type router struct {
	*httprouter.Router
}

func (r *router) Get(path string, handler http.Handler) {
	r.GET(path, wrapHandler(handler))
}

func (r *router) Post(path string, handler http.Handler) {
	r.POST(path, wrapHandler(handler))
}

func (r *router) Put(path string, handler http.Handler) {
	r.PUT(path, wrapHandler(handler))
}

func (r *router) Delete(path string, handler http.Handler) {
	r.DELETE(path, wrapHandler(handler))
}

func wrapHandler(h http.Handler) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		ctx := r.Context()
		h.ServeHTTP(w, r.WithContext(context.WithValue(ctx, "params", ps)))
	}
}

func NewRouter() *router {
	return &router{httprouter.New()}
}

type Server struct {
	router         *router
	listener       net.Listener
	consulClient   *api.Client
	tasksCollector *tasks.Collector
}

func (s *Server) Shutdown() {
	if s != nil {
		log.Printf("Shutting down http server")
		s.listener.Close()
	}
}

func NewServer(client *api.Client) (*Server, error) {
	var listener net.Listener
	var err error
	if listener, err = net.Listen("tcp", ":8800"); err != nil {
		return nil, err
	}
	httpServer := &Server{
		router:         NewRouter(),
		listener:       listener,
		consulClient:   client,
		tasksCollector: tasks.NewCollector(client),
	}

	httpServer.registerHandlers()
	log.Printf("Starting HTTPServer on port 8800")
	go http.Serve(httpServer.listener, httpServer.router)

	return httpServer, nil
}

func (s *Server) registerHandlers() {
	commonHandlers := alice.New(loggingHandler, recoverHandler)
	s.router.Post("/deployments", commonHandlers.Append(contentTypeHandler("application/zip")).ThenFunc(s.newDeploymentHandler))
	s.router.Delete("/deployments/:id", commonHandlers.ThenFunc(s.deleteDeploymentHandler))
	s.router.Get("/deployments/:id", commonHandlers.Append(acceptHandler("application/json")).ThenFunc(s.getDeploymentHandler))
	s.router.Get("/deployments", commonHandlers.Append(acceptHandler("application/json")).ThenFunc(s.listDeploymentsHandler))
	s.router.Get("/deployments/:id/events", commonHandlers.Append(acceptHandler("application/json")).ThenFunc(s.pollEvents))
	s.router.Get("/deployments/:id/node/:name/events", commonHandlers.Append(acceptHandler("application/json")).ThenFunc(s.pollNodeEvents))
}
