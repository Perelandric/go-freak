package freak

import (
	"bytes"
	"fmt"
	"net/http"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

type Route struct {
	Path        string
	Component   *component
	Catch404    bool
	DisplayName string
	Description string
}

func NewServer(host string, port uint16, compressionLevel int) *server {
	const invalid = "%d is an invalid %s. Using %d instead.\n"

	var s = server{
		host:             host,
		port:             strconv.Itoa(int(port)),
		routes:           map[string]*freakHandler{},
		compressionLevel: compressionLevel,
	}

	// TODO: Redo comporession levels
	if s.compressionLevel < -2 {
		fmt.Printf(invalid, s.compressionLevel, "compression", -2)
		s.compressionLevel = -2

	} else if s.compressionLevel > 9 {
		fmt.Printf(invalid, s.compressionLevel, "compression", 9)
		s.compressionLevel = 9
	}

	return &s
}

type server struct {
	host, port string

	routes map[string]*freakHandler

	tailRoutesMux sync.RWMutex
	tailRoutes    map[string]*freakHandler

	compressionLevel int
	binaryPath       string // Path leading to the application binary's directory

	js, css         bytes.Buffer // TODO: I think these will eventually be a per-root-route buffer
	cssPath, jsPath string

	isStarted bool
}

func (s *server) SetRoutes(routes ...Route) {
	if s.isStarted {
		// TODO: Log message
		return
	}

	for _, route := range routes {
		if route.Component == nil {
			// TODO: Log message
			continue
		}

		fmt.Printf("Adding route: %q\n", route.Path)

		if !tailHandlersExist && route.Catch404 {
			tailHandlersExist = true
		}

		var sh = freakHandler{
			route:            route,
			staticFilePath:   "",
			routePathNoSlash: route.Path,
		}

		if route.Path[len(route.Path)-1] == '/' {
			sh.routePathNoSlash = route.Path[0 : len(route.Path)-1]
		}

		sh.siteMapNode = newSiteMapNode(route.Path, &sh.route)

		// TODO: will need scripts/css/whatever

		s.routes[dropTrailingSlash(route.Path)] = &sh
	}
}

func (s *server) Start(host string, port uint16) error {
	if s.isStarted {
		return nil
	}
	s.isStarted = true

	fmt.Println("Starting server...")

	// All fragments should have been initialized, so we have a master list of
	// pointers to all the static HTML.
	locateSubstrings()

	fmt.Println("Working directory:", s.binaryPath)

	// Add routes to mandatory root-based resources
	for _, pth := range []string{"/sitemap.xml", "/favicon.ico", "/robots.txt"} {
		s.routes[pth] = &freakHandler{staticFilePath: pth}
	}

	rootRoute = s.routes["/"]

	var addr = s.host + ":" + s.port

	fmt.Printf("\nStarting server on %s\n", addr)

	return http.ListenAndServe(addr, s)
}

type freakHandler struct {
	route Route

	siteMapNode *SiteMapNode

	routePathNoSlash string // no trailing slash

	staticFilePath string

	dataSmashRouteId int32
}

// TODO: What is this for?
func (_ *freakHandler) staticFileHandler(r http.ResponseWriter, req *http.Request, pth string) {
	http.ServeFile(r, req, pth)
}

var tailHandlersExist bool

func (s *server) addTailRoute(sh *freakHandler, fullPth string) {
	s.tailRoutesMux.Lock()

	s.tailRoutes[dropTrailingSlash(fullPth)] = sh

	s.tailRoutesMux.Unlock()
}

func (s *server) getTailRoute(pth string) (*freakHandler, int) {
	s.tailRoutesMux.RLock()
	var sh = s.tailRoutes[pth]
	s.tailRoutesMux.RUnlock()

	if sh != nil {
		return sh, len(sh.routePathNoSlash)
	}
	return nil, -1
}

func dropTrailingSlash(pth string) string {
	if len(pth) > 1 && pth[len(pth)-1] == '/' {
		return pth[0 : len(pth)-1]
	}
	return pth
}

// Avoids a map lookup
var rootRoute *freakHandler

func (s *server) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	var urlPath = req.URL.Path

	// TODO: I think I should have a separate server for resources

	// Check for static resource request
	if len(urlPath) >= 5 &&
		urlPath[1] == 'r' &&
		urlPath[2] == 'e' &&
		urlPath[3] == 's' &&
		urlPath[4] == '/' {
		s.serveFile(resp, req, urlPath)
		return
	}

	urlPath = cleanPath(urlPath)
	req.URL.Path = urlPath

	if urlPath == "/" {
		s.serve(resp, req, urlPath, -1, rootRoute, false)
		return
	}

	sh := s.routes[urlPath]

	if sh != nil {
		if len(sh.staticFilePath) != 0 {
			http.ServeFile(resp, req, filepath.Join(s.binaryPath, sh.staticFilePath))
		} else {
			s.serve(resp, req, urlPath, -1, sh, false)
		}
		return
	}

	// Check if the root or any of its sub-routes allow a tail that would
	// cause the URL to not be found in routes
	if !tailHandlersExist {
		http.NotFound(resp, req)
		return
	}

	var testPath = urlPath
	var tailIdx = -1

	for {
		sh, tailIdx = s.getTailRoute(testPath)
		if sh != nil {
			s.serve(resp, req, urlPath, tailIdx, sh, true)
			return
		}

		var lastSlash = strings.LastIndexByte(testPath, '/')

		if lastSlash == 0 { // We're down to the root
			if rootRoute.route.Catch404 {
				s.serve(resp, req, urlPath, 1, rootRoute, false)
			} else {
				http.NotFound(resp, req)
			}
			return
		}

		testPath = testPath[0:lastSlash] // shorten until (and excluding) the last '/'

		sh = s.routes[testPath]
		if sh != nil && sh.route.Catch404 {
			s.serve(resp, req, urlPath, len(testPath), sh, false)
			return
		}
	}
}

func (s *server) serve(
	resp http.ResponseWriter,
	req *http.Request,
	fullPth string,
	tailIdx int,
	fh *freakHandler,
	tailWasCached bool,
) {

	var respHdrs = resp.Header()
	respHdrs[_contentType] = htmlContentHeader

	var doGzip = strings.Contains(req.Header.Get(_acceptEncoding), _gzip) &&
		!strings.Contains(req.Header.Get(_userAgent), _msie6)

	if doGzip {
		respHdrs[_contentEncoding] = gzipHeader
	}

	var r = s.getResponse(resp, req, fh.siteMapNode, doGzip)
	defer s.putResponse(r)

	r.thisAsValue = reflect.ValueOf(&r)

	r.do(fh.route.Component, &RouteData{})

	if r.state.has(sent) {
		// TODO: Need to actually be handling HTTP error types
		return
	}

	var hasTail = tailIdx != -1
	if hasTail && !tailWasCached && r.state.has(cacheTail) {
		s.addTailRoute(fh, fullPth)
	}
}

var errNilComponent = fmt.Errorf("handler returned a nil component")

const (
	_poolEnabled = true
	_bufMaxSize  = 50000
)

var _poolSize = 4 * runtime.NumCPU()

var respPool = make(chan *Response, _poolSize)

//var allocated = 0

func (s *server) getResponse(
	resp http.ResponseWriter,
	req *http.Request,
	node *SiteMapNode,
	doGzip bool,
) (r *Response) {

	if _poolEnabled {
		select {
		case r = <-respPool:

		default:
			r = newResponse(s.compressionLevel, _bufMaxSize)
			//allocated++
			//fmt.Printf("allocated %d response objects\n", allocated)
		}
	} else {
		r = newResponse(s.compressionLevel, _bufMaxSize)
	}

	if doGzip {
		r.state.set(acceptsGzip)

		r.gzip.Reset(&r.buf)
		r.writer = &r.gzip

	} else {
		r.writer = &r.buf
	}

	r.siteMapNode = node

	return r
}

// putResponse puts the *Response object back in the pool.
func (s *server) putResponse(r *Response) {
	if !r.state.has(sent) {
		r.resp.WriteHeader(http.StatusOK)

		if r.state.hasAny(acceptsGzip) {
			r.gzip.Close()
			r.gzip.Reset(nil)
		}

		r.resp.Write(r.buf.Bytes())
	}

	if r.buf.Cap() > _bufMaxSize {
		r.buf = *bytes.NewBuffer(r.buf.Bytes()[0:0:_bufMaxSize])
	} else {
		r.buf.Reset()
	}

	// Clear data and put back into the pool.
	r.cookiesToSend = r.cookiesToSend[0:0]
	r.resp = nil
	r.req = nil
	r.state = state{}

	if _poolEnabled {
		select {
		case respPool <- r: // Successfully placed back into pool

		default: // let overflow get GC'd
		}
	}
}

func cleanPath(urlPath string) string {
	if len(urlPath) < 2 || urlPath[0] != '/' {
		return "/"
	}

	var start = 1
	for start < len(urlPath) && urlPath[start] == '/' {
		start++
	}

	// TODO: If last byte is not '/', should I add it?
	var end = len(urlPath) - 1
	for urlPath[end] == '/' && end > start {
		end--
	}

	urlPath = urlPath[start-1 : end+1]

	var b []byte
	var idx = 1

	for idx < len(urlPath) {

		// TODO: Why am I returning ERROR in this case?
		if urlPath[idx] == '.' {
			return "ERROR"
		}

		if urlPath[idx] == '/' {
			idx++

			if b == nil {
				b = make([]byte, 0, len(urlPath))
			}
			b = append(b, urlPath[0:idx]...)

			for idx < len(urlPath) && urlPath[idx] == '/' {
				idx++
			}

			urlPath = urlPath[idx:]
			idx = 0

			if urlPath[idx] == '.' {
				return "ERROR"
			}
		}

		var newIdx = strings.IndexByte(urlPath[idx:], '/')
		if newIdx == -1 {
			break
		}
		idx += newIdx
	}

	if b == nil {
		return urlPath
	}
	return string(append(b, urlPath...))
}
