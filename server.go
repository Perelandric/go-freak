package freak

import (
	"net/http"
	"strconv"
)

type Route struct {
	Path      string
	Component *component
}

type Server struct {
	host, port string
}

func (s *Server) SetRoutes(routes ...Route) {
	for _, route := range routes {
		if route.Component == nil {
			continue
		}

		http.Handle(route.Path, route.Component)
	}
}

func (s *Server) Start(host string, port uint16) {
	s.host = host
	s.port = strconv.Itoa(int(port))

	http.ListenAndServe(s.host+":"+s.port, nil)
}
