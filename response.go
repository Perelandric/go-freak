package freak

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"reflect"
	"runtime"
)

type state struct {
	flags stateFlag
}
type stateFlag uint8

func (s *state) set(flags stateFlag) {
	s.flags |= flags
}
func (s *state) unset(flags stateFlag) {
	s.flags &^= flags
}

func (s state) has(flags stateFlag) bool {
	return s.flags&flags == flags
}
func (s state) hasAny(flags stateFlag) bool {
	return s.flags&flags != 0
}

const (
	// TODO: Review these flags`
	sent = 1 << stateFlag(iota)
	acceptsGzip
	cacheTail
	skipElement
	skipContent
	allStatic
	allSkip
)

type RouteData struct {
}

type response struct {
	// Unable to write directly to the ResponseWriter (or via gzip) because
	// that cause WriteHeader to take place with StatusOK, which means we can
	// no longer redirect.
	// So instead we must write to a bytes.Buffer.
	buf bytes.Buffer

	// When gzipping is enabled, the &buf in this struct becomes
	// the underlying Writer for the gzip.Writer
	gzip gzip.Writer

	Response
}

type Response struct {
	// This is for calling the provided callbacks via reflection.
	// It holds a circular reference to itself.
	thisAsValue reflect.Value

	// This receives either the &buf or the &gzip from this struct.
	// ONLY write to this writer, not to 'buf' or 'gzip'
	writer io.Writer

	siteMapNode *SiteMapNode // for the requested page

	quickZero
}

type quickZero struct {
	cookiesToSend      []*http.Cookie
	wrapperEndingFuncs []func(*Response)
	resp               http.ResponseWriter
	req                *http.Request
	state              state
}

const (
	_poolEnabled = true
	_bufMaxSize  = 50000
)

var _poolSize = 4 * runtime.NumCPU()

var respPool = make(chan *response, _poolSize)

//var allocated = 0

func getResponse(
	s *server,
	resp http.ResponseWriter,
	req *http.Request,
	node *SiteMapNode,
	doGzip bool,
) (r *response) {

	if _poolEnabled {
		select {
		case r = <-respPool:
			goto INITIALIZE

		default:
			// need default to handle an empty pool
		}
	}

	{ // create new Response
		gz, err := gzip.NewWriterLevel(nil, s.compressionLevel)
		if err != nil {
			panic(err) // unreachable, because s.compressionLevel was already checked
		}

		r = &response{
			gzip: *gz,
			buf:  *bytes.NewBuffer(make([]byte, 0, _bufMaxSize)),
		}
		r.thisAsValue = reflect.ValueOf(&r.Response)
	}

INITIALIZE:
	r.req = req
	r.resp = resp
	r.siteMapNode = node

	if doGzip {
		r.state.set(acceptsGzip)
		r.gzip.Reset(&r.buf)
		r.writer = &r.gzip

	} else {
		r.writer = &r.buf
	}

	return r
}

// putResponse puts the *Response object back in the pool.
func putResponse(s *server, r *response) {
	if !r.state.has(sent) {
		r.resp.WriteHeader(http.StatusOK)

		if r.state.has(acceptsGzip) {
			r.gzip.Close()
		}

		r.resp.Write(r.buf.Bytes())
	}

	r.buf.Reset()

	if r.buf.Cap() > _bufMaxSize {
		// Reduce underlying capacity to the given maximum
		r.buf = *bytes.NewBuffer(r.buf.Bytes()[0:0:_bufMaxSize])
	}

	// Clear data and put back into the pool.
	r.quickZero = quickZero{
		cookiesToSend: r.cookiesToSend[0:0],
	}

	if _poolEnabled {
		select {
		case respPool <- r: // Successfully placed back into pool

		default:
			// let overflow get GC'd
		}
	}
}

func (r *Response) WriteBytes(b []byte) {
	writeEscapeHTMLBytes(r.writer, b)
}

func (r *Response) WriteString(s string) {
	writeEscapeHTMLString(r.writer, s)
}

func (r *Response) WriteStringNoEscape(s string) {
	r.writer.Write(strToBytes(s))
}

func (r *Response) WriteBytesNoEscape(b []byte) {
	r.writer.Write(b)
}

func (r *Response) DoComponent(c *component, dataI interface{}) {
	r.do(c, dataI)
}

type WrapperResponse struct {
	r *Response
}

func (wr *WrapperResponse) DoWrapper(w *wrapper, dataI interface{}) {
	var r = wr.r

	if w == nil || r.state.has(sent) {
		return
	}

	// Originally provided by the calling component
	var temp = r.wrapperEndingFuncs
	r.wrapperEndingFuncs = nil

	r.do(&w.preContent, dataI)

	r.wrapperEndingFuncs = append(temp, func(r *Response) {
		r.do(&w.postContent, dataI)
	})
}

func (wr *WrapperResponse) SkipContent() {
	wr.r.state.set(skipContent)
}

/*

// Send503 sends a `StatusServiceUnavailable` response.
func (r *Response) Send503(err error) {
	log.Println(err)

	if !r.wasSent() {
		r.state.set(sent)
		r.response.WriteHeader(http.StatusServiceUnavailable)
	}
}

// Redirect sends a redirect with the given response `code` to the given `url`.
func (r *Response) Redirect(code int, url string) {
	r.doRedirect(code, url)
}

// TemporaryRedirect sends a `StatusTemporaryRedirect` (307) response.
func (r *Response) TemporaryRedirect(url string) {
	r.doRedirect(http.StatusTemporaryRedirect, url)
}

// RedirectToGet sends a `StatusSeeOther` (303) response to redirect a POST to
// a GET request.
func (r *Response) RedirectToGet(url string) {
	r.doRedirect(http.StatusSeeOther, url)
}

// PermanentRedirect sends a `StatusMovedPermanently` (301) response.
func (r *Response) PermanentRedirect(url string) {
	r.doRedirect(http.StatusMovedPermanently, url)
}

func (r *Response) doRedirect(code int, url string) {
	if !r.wasSent() {
		r.state.set(sent)

		//		r.sendCookies()

		http.Redirect(r.response, r.request, url, code)
	}
}

// SetCookie sets the given cookie to be sent with the response.
func (br *baseResponse) SetCookie(c *http.Cookie) {
	http.SetCookie(br.response, c)
}

// GetCookie gets the cookie from the current request.
func (br *baseResponse) GetCookie(name string) *http.Cookie {
	c, err := br.request.Cookie(name)
	if err != nil && err != http.ErrNoCookie {
		log.Printf("GetCookie error: %q\n", err)
	}

	return c // Return the Cookie or nil
}

// ExpireCookie expires the given cookie.
func (br *baseResponse) ExpireCookie(c *http.Cookie) {
	cc := *c
	cc.MaxAge = -1
	http.SetCookie(br.response, &cc)
}

*/

func (r *Response) do(c *component, dataI interface{}) {
	if r.state.has(sent) {
		return
	}

	var callArgs = [2]reflect.Value{r.thisAsValue, reflect.ValueOf(dataI)}

	var wrapperEndStack [][]func(*Response)
	var endStackIndex = -1

	if c.maxWrapperNesting != 0 {
		wrapperEndStack = getWrapEndingSliceStack(c.maxWrapperNesting)
		defer returnWrapEndingSliceStack(wrapperEndStack)
	}

	for i := 0; i < len(c.markers); i++ {
		var m = c.markers[i]

		r.writer.Write(m.htmlPrefix)

		switch m.kind {
		case plainMarker:
			m.callback.Call(callArgs[:])
			if r.state.has(sent) {
				return
			}

		case wrapperStartMarker:
			endStackIndex++

			if endStackIndex >= len(wrapperEndStack) {
				panic("unreachable")
			}

			var funcSlice = wrapperEndStack[endStackIndex]
			r.wrapperEndingFuncs = funcSlice[0:0:cap(funcSlice)]

			m.callback.Call(callArgs[:])

			if r.state.has(sent) {
				return
			}

			// In case the end-funcs slice was grown beyond its original capacity
			wrapperEndStack[endStackIndex] = r.wrapperEndingFuncs

			r.wrapperEndingFuncs = nil

			if !r.state.has(skipContent) {
				continue
			}

			r.state.unset(skipContent)

			// We're skipping to the ending, so set `i` and `m`, and fallthrough so
			// that we're not writing the htmlPrefix, since it's part of the content
			i = int(m.wrapperEndIndex)
			m = c.markers[i]
			fallthrough

		case wrapperEnd:
			if len(wrapperEndStack) == 0 {
				panic("unreachable")
			}

			var funcSlice = wrapperEndStack[endStackIndex]

			for i := len(funcSlice) - 1; i != -1 && !r.state.has(sent); i-- {
				funcSlice[i](r)
			}

			if r.state.has(sent) {
				return
			}

			endStackIndex--

		default:
			panic("unreachable")
		}
	}

	if endStackIndex != -1 {
		panic("unreachable")
	}

	r.writer.Write(c.htmlTail)
}
