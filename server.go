package freak

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
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

func NewServer(host string, port uint16, compressionLevel int) (*server, error) {
	const invalid = "%d is an invalid %s. Using %d instead.\n"

	var s = server{
		host:             host,
		port:             strconv.Itoa(int(port)),
		routes:           map[string]*freakHandler{},
		compressionLevel: compressionLevel,
	}

	var err error

	s.binaryPath, err = filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		return nil, err
	}

	// TODO: Redo comporession levels
	if s.compressionLevel < -2 {
		fmt.Printf(invalid, s.compressionLevel, "compression", -2)
		s.compressionLevel = -2

	} else if s.compressionLevel > 9 {
		fmt.Printf(invalid, s.compressionLevel, "compression", 9)
		s.compressionLevel = 9
	}

	return &s, nil
}

type server struct {
	host, port string

	routes map[string]*freakHandler

	tailRoutesMux sync.RWMutex

	tailRoutes map[string]*freakHandler

	compressionLevel int
	binaryPath       string // Path leading to the application binary's directory

	js, css         bytes.Buffer // TODO: I think these will eventually be a per-root-route buffer
	cssPath, jsPath string

	isStarted bool
}

// Avoids a map lookup
var rootRoute *freakHandler

func (s *server) SetRoutes(routes ...Route) error {
	if s.isStarted {
		return fmt.Errorf("Server is already running")
	}

	for _, route := range routes {
		if route.Component == nil {
			return fmt.Errorf("A route's component must not be nil")
		}

		var pth = cleanPath(route.Path)

		route.Path = pth

		fmt.Printf("Adding route: %q\n", pth)

		if route.Catch404 {
			// TODO: Maybe have a separate tailRoutes map for each Route that accepts them.
			//		Then they can handle their own periodic maintenance, like clearing.
			tailHandlersExist = true
		}

		var sh = freakHandler{
			route:          route,
			staticFilePath: "",
		}

		sh.siteMapNode = newSiteMapNode(pth, &sh.route)

		// TODO: will need scripts/css/whatever

		var err = s.setRoutePaths(pth, &sh)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *server) setRoutePaths(pth string, fh *freakHandler) error {
	if len(pth) == 0 {
		pth = "/"
		fh.route.Path = pth
	}

	if _, ok := s.routes[pth]; ok {
		return fmt.Errorf("Path %q defined more than once", pth)
	}

	if pth == "/" {
		s.routes[pth] = fh
		rootRoute = fh
		return nil
	}

	if !strings.HasSuffix(pth, "/") {
		pth = pth + "/"
		fh.route.Path = pth
	}

	// set the path both with and without trailing slash
	s.routes[pth] = fh
	s.routes[pth[0:len(pth)-1]] = fh

	return nil
}

func (s *server) Start(host string, port uint16) error {
	if s.isStarted {
		// TODO: Log message
		return nil
	}
	s.isStarted = true

	fmt.Println("Starting server...")

	// All fragments should have been initialized, so we have a master list of
	// pointers to all the static HTML.
	locateSubstrings() // TODO: Implement this..........

	fmt.Println("Working directory:", s.binaryPath)

	// Add routes to mandatory root-based resources
	for _, pth := range []string{"/sitemap.xml", "/favicon.ico", "/robots.txt"} {
		s.routes[pth] = &freakHandler{staticFilePath: pth}
	}

	var addr = s.host + ":" + s.port

	fmt.Printf("\nStarting server on %s\n", addr)

	return http.ListenAndServe(addr, s)
}

type freakHandler struct {
	route Route

	siteMapNode *SiteMapNode

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

	s.tailRoutes[pathNoTrailingSlash(fullPth)] = sh

	s.tailRoutesMux.Unlock()
}

func (s *server) getTailRoute(pth string) (*freakHandler, int) {
	s.tailRoutesMux.RLock()
	var sh = s.tailRoutes[pth]
	s.tailRoutesMux.RUnlock()

	if sh != nil {
		return sh, len(sh.route.Path) - 1
	}
	return nil, -1
}

func pathNoTrailingSlash(pth string) string {
	if len(pth) > 1 && pth[len(pth)-1] == '/' {
		return pth[0 : len(pth)-1]
	}
	return pth
}

func (s *server) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	var urlPath = req.URL.Path

	if urlPath == "/" {
		s.serve(resp, req, urlPath, -1, rootRoute, false)
		return
	}

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

	fh := s.routes[urlPath]

	if fh == nil {
		urlPath = cleanPath(urlPath)
		req.URL.Path = urlPath

		fh = s.routes[urlPath]
	}

	if fh != nil {
		if len(fh.staticFilePath) != 0 {
			http.ServeFile(resp, req, filepath.Join(s.binaryPath, fh.staticFilePath))
		} else {
			s.serve(resp, req, urlPath, -1, fh, false)
		}
		return
	}

	const IMPLEMENT_TAIL_HANDLERS_LATER = true

	// Check if the root or any of its sub-routes allow a tail that would
	// cause the URL to not be found in routes
	if IMPLEMENT_TAIL_HANDLERS_LATER || !tailHandlersExist {
		http.NotFound(resp, req)
		return
	}

	var testPath = urlPath
	var tailIdx = -1

	for {
		fh, tailIdx = s.getTailRoute(testPath)
		if fh != nil {
			s.serve(resp, req, urlPath, tailIdx, fh, true)
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

		fh = s.routes[testPath]
		if fh != nil && fh.route.Catch404 {
			s.serve(resp, req, urlPath, len(testPath), fh, false)
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
	// Any path not starting with `/` goes to home page
	if len(urlPath) == 0 {
		return "/"
	}

	var prevSlashIndex = 0
	var idx = 1
	var b []byte

	for idx < len(urlPath) {
		// Here idx always at the character after a slash

		{ // Skip over multiple adjacent slashes /foo///bar becomes /foo/bar
			var temp = idx
			for urlPath[idx] == '/' {
				if idx++; idx == len(urlPath) {
					if b == nil {
						return urlPath[0:temp]
					}
					return string(b)
				}
			}

			if temp != idx {
				if b == nil { // TODO: Get from pool
					b = make([]byte, 0, len(urlPath)-(idx-temp))
				}
				b = append(b, urlPath[prevSlashIndex:temp-1]...)
				prevSlashIndex = idx - 1
			}
		}

		var newIdx = strings.IndexByte(urlPath[idx:], '/')
		if newIdx == -1 {
			if urlPath[len(urlPath)-1] != '/' {
				if b != nil {
					b = append(b, '/')
				} else {
					urlPath += "/"
				}
			}
			break
		}

		idx += newIdx + 1
	}

	if b == nil {
		return urlPath
	}
	return string(append(b, urlPath[prevSlashIndex:]...))
}
