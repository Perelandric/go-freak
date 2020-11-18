package freak

import (
	"net/http"
	"reflect"
	"strconv"
)

type Server struct {
	host, port string
	isStarted  bool
}

func (s *Server) SetRoutes(routes ...Route) {
	if s.isStarted {
		// TODO: Log message
		return
	}

	for _, route := range routes {
		if route.Component == nil {
			continue
		}

		http.Handle(route.Path, (*componentServer)(route.Component))
	}
}

func (s *Server) Start(host string, port uint16) {
	s.host = host
	s.port = strconv.Itoa(int(port))
	s.isStarted = true

	http.ListenAndServe(s.host+":"+s.port, nil)
}

type Route struct {
	Path      string
	Component *component
}

type componentServer component

func (cs *componentServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var resp = Response{
		req:  r,
		resp: w,
		buf:  make([]byte, 0, 512),
	}

	resp.thisAsValue = reflect.ValueOf(&resp)

	resp.do((*component)(cs), &RouteData{})

	if resp.halt {
		return
	}

	w.Write(resp.buf)
}
