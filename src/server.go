package freak

import (
	_ "embed"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	static "github.com/Perelandric/static-serve"
)

const _res_dir_name = "res"
const _res_url_path = "/" + _res_dir_name + "/"

type RouteData struct {
	Path        string
	DisplayName string
	Description string
}

type Route struct {
	RouteData
	Handler  func(*RouteResponse, *RouteData)
	Catch404 bool
}

type Server server

func (s *Server) SetRoutes(routes ...Route) error {
	return (*server)(s).setRoutes(routes)
}

func (s *Server) Start() error {
	return (*server)(s).start()
}
func (s *Server) MustStart() {
	var err = s.Start()
	if err != nil {
		panic(err)
	}
}

func NewServer(host string, port uint16, compressionLevel int) (*Server, error) {
	return newServer(host, port, compressionLevel)
}

type server struct {
	host, port string

	routes map[string]*freakHandler

	tailRoutesMux sync.RWMutex

	tailRoutes map[string]*freakHandler

	compressionLevel int
	binaryPath       string // Path leading to the application binary's directory

	css, js *os.File

	isStarted bool

	static_server static.StaticServe
}

func newServer(host string, port uint16, compressionLevel int) (*Server, error) {
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

	err = os.Mkdir(filepath.Join(s.binaryPath, _res_dir_name), os.ModeDir)
	if err != nil && !os.IsExist(err) {
		return nil, err
	}

	static_server, err := static.NewStaticServer(s.binaryPath)
	if err != nil {
		return nil, err
	}

	s.static_server = *static_server

	err = s.writeCssAndJs()
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

	return (*Server)(&s), nil
}

// Avoids a map lookup
var rootRoute *freakHandler

func (s *server) setRoutes(routes []Route) error {
	if s.isStarted {
		return fmt.Errorf("Server is already running")
	}

	for _, route := range routes {
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

func (s *server) writeCssAndJs() (err error) {

	var cssFullPath = filepath.Join(s.binaryPath, _cssInsertionPath)
	var jsFullPath = filepath.Join(s.binaryPath, _jsInsertionPath)

	s.css, err = os.Create(cssFullPath)
	if err != nil {
		return err
	}
	defer s.css.Close()

	_, err = s.css.Write(allCss.Bytes())
	if err != nil {
		return err
	}

	s.js, err = os.Create(jsFullPath)
	if err != nil {
		return err
	}
	defer s.js.Close()

	_, err = fmt.Fprintf(
		s.js,
		`var freak={loaders:new Map([%s]),ctors:new Map()};%s`,
		allJs.String(),
		jslib,
	)
	return err
}

//go:embed lib.js
var jslib string

func (s *server) start() error {
	if s.isStarted {
		// TODO: Log message
		return nil
	}

	defer os.Remove(s.css.Name())
	defer os.Remove(s.js.Name())

	s.isStarted = true

	fmt.Println("Starting server...")

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
	if len(urlPath) >= len(_res_url_path) &&
		urlPath[0] == _res_url_path[0] &&
		urlPath[1] == _res_url_path[1] &&
		urlPath[2] == _res_url_path[2] &&
		urlPath[3] == _res_url_path[3] &&
		urlPath[4] == _res_url_path[4] {
		s.static_server.ServeHTTP(resp, req)
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
			// TODO: Why isn't the 'binaryPath' already joined?``
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

	var doGzip = s.compressionLevel != 0 &&
		strings.Contains(req.Header.Get(_acceptEncoding), _gzip)

	if doGzip {
		respHdrs[_contentEncoding] = gzipHeader
	}

	var r = getResponse(s, resp, req, fh.siteMapNode, doGzip)
	defer putResponse(s, r)

	fh.route.Handler(&RouteResponse{r: r.response}, &RouteData{})

	if r.responseState.has(sent) {
		// TODO: Need to actually be handling HTTP error types
		return
	}

	var hasURLTail = tailIdx != -1
	if hasURLTail && !tailWasCached && r.responseState.has(cacheTail) {
		s.addTailRoute(fh, fullPth)
	}
}

var errNilComponent = fmt.Errorf("handler returned a nil component")

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
