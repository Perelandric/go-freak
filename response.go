package freak

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"runtime"
)

type state[T responseStateFlag | componentStateFlag] struct {
	flags T
}

type responseStateFlag uint8

func (s *state[T]) set(flags T) {
	s.flags |= flags
}
func (s *state[T]) unset(flags T) {
	s.flags &^= flags
}

func (s state[T]) has(flags T) bool {
	return s.flags&flags == flags
}
func (s state[T]) hasAny(flags T) bool {
	return s.flags&flags != 0
}

const (
	// TODO: Review these flags`
	sent = 1 << responseStateFlag(iota)
	acceptsGzip
	cacheTail
	allStatic
	allSkip
)

type componentStateFlag uint8

const (
	skipElement = 1 << componentStateFlag(iota)
	skipContent
)

type responseBase[T any] struct {
	// Unable to write directly to the ResponseWriter (or via gzip) because
	// that cause WriteHeader to take place with StatusOK, which means we can
	// no longer redirect.
	// So instead we must write to a bytes.Buffer.
	buf bytes.Buffer

	// When gzipping is enabled, the &buf in this struct becomes
	// the underlying Writer for the gzip.Writer
	gzip gzip.Writer

	response[T]
}

// Response is the part that we actually pass through to the components
type response[T any] struct {
	// This receives either the &buf or the &gzip from this struct.
	// ONLY write to this writer, not to 'buf' or 'gzip'
	writer io.Writer

	siteMapNode *SiteMapNode // for the requested page

	quickZero[T]
}

type quickZero[T any] struct {
	cookiesToSend  []*http.Cookie
	wrapperEndings func()
	resp           http.ResponseWriter
	req            *http.Request
	responseState  state[responseStateFlag]
	componentState state[componentStateFlag]
}

func (r *response[T]) SkipElement() {
	r.componentState.set(skipElement)
}

func (r *response[T]) SkipContent() {
	r.componentState.set(skipContent)
}

func (r *response[T]) WriteBytes(b []byte) {
	writeEscapeHTMLBytes(r.writer, b)
}

func (r *response[T]) WriteString(s string) {
	writeEscapeHTMLString(r.writer, s)
}

func (r *response[T]) WriteStringNoEscape(s string) {
	r.writer.Write(strToBytes(s))
}

func (r *response[T]) WriteBytesNoEscape(b []byte) {
	r.writer.Write(b)
}

func (r *response[T]) Insert(c *component[T], data T) {
	r.insert(c, data, nil)
}

type PreResponse[T any] struct {
	*response[T]
}

type AttrResponse[T any] struct {
	r *response[T]
}

var (
	bytSpaceUnderscoreEqualDblQuote = []byte(` _="`)
	bytSpace                        = []byte{' '}
	bytEqualDblQuote                = []byte{'=', '"'}
)

func (r AttrResponse[T]) AddAttr(key, val string) {
	writeAttr[T](r.r.writer, key, val, true)
}
func (r AttrResponse[T]) AddAttrNoEscape(key, val string) {
	writeAttr[T](r.r.writer, key, val, false)
}
func writeAttr[T any](w io.Writer, key, val string, escape bool) {
	if len(key) == 0 {
		w.Write(bytSpaceUnderscoreEqualDblQuote)
		goto WRITE_VAL
	}

	w.Write(bytSpace)
	if escape {
		writeEscapeHTMLString(w, key)
	} else {
		w.Write(strToBytes(key))
	}
	w.Write(bytEqualDblQuote)

WRITE_VAL:
	writeUnDoubleQuote(w, val)
	w.Write(bytDblQuot)
}

func (r AttrResponse[T]) SkipContent() {
	r.r.SkipContent()
}

type PostResponse[T any] struct {
	r *response[T]
}

func (r PostResponse[T]) SkipContent() {
	r.r.SkipContent()
}

const (
	_poolEnabled = true
	_bufMaxSize  = 50000
)

var _poolSize = 4 * runtime.NumCPU()

var respPool = make(chan *responseBase[*RouteData], _poolSize)

//var allocated = 0

func getResponse(
	s *server,
	resp http.ResponseWriter,
	req *http.Request,
	node *SiteMapNode,
	doGzip bool,
) (r *responseBase[*RouteData]) {

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

		r = &responseBase[*RouteData]{
			gzip: *gz,
			buf:  *bytes.NewBuffer(make([]byte, 0, _bufMaxSize)),
		}
	}

INITIALIZE:
	r.req = req
	r.resp = resp
	r.siteMapNode = node

	if doGzip {
		r.responseState.set(acceptsGzip)
		r.gzip.Reset(&r.buf)
		r.writer = &r.gzip

	} else {
		r.writer = &r.buf
	}

	return r
}

// putResponse puts the *Response object back in the pool.
func putResponse(s *server, r *responseBase[*RouteData]) {
	if !r.responseState.has(sent) {
		r.resp.WriteHeader(http.StatusOK)

		if r.responseState.has(acceptsGzip) {
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
	r.quickZero = quickZero[*RouteData]{
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

func (r *response[T]) insert(c *component[T], data T, newlyReceivedEndings []wrapperEndingAndIndex) {
	if c == nil || r.responseState.has(sent) {
		return
	}

	var isWrapper = c.wrapperContentMarkerIndex != -1
	var doingWrapperEnding = isWrapper && newlyReceivedEndings != nil

	var finishComponent = !isWrapper || doingWrapperEnding

	// Endings should be empty by the time we're at the end of the component, but if the
	// component is a wrapper, they need to be carried over to be completed in the second half.
	var mostRecentEnding wrapperEndingAndIndex

	defer func(prevAdjacentEndings func(), flags componentStateFlag) {

		if mostRecentEnding.index != 0 {
			newlyReceivedEndings = append(newlyReceivedEndings, mostRecentEnding)
		}

		if DEBUG && finishComponent && len(newlyReceivedEndings) != 0 {
			panic("unreachable") // For a non-wrapper, the received endings should have been placed
		}

		r.componentState.flags = flags

		if isWrapper {

			r.wrapperEndings = func() { // <-- Callback to execute this wrapper's ending

				if newlyReceivedEndings == nil {
					// Must not be 'nil'; it indicates that we're doing the second part of a wrapper
					newlyReceivedEndings = []wrapperEndingAndIndex{}
				}

				r.insert(c, data, newlyReceivedEndings) // Completes the second half of the wrapper

				if prevAdjacentEndings != nil {
					prevAdjacentEndings() // If there were adjacent wrappers applied, the next one is completed
				}
			}

		} else {
			r.wrapperEndings = prevAdjacentEndings
		}

	}(r.wrapperEndings, r.componentState.flags)
	// ----^^^^^^------------^^^^^^--- capture their current value

	var htmlIndex uint16 = 0

	r.wrapperEndings = nil

	// Helper function to write the HTML until the given target index, which is usually
	//	the start of a new marker, the end of a component or first half of a wrapper. It
	//	checks for any receved "wrapper endings" in the section of HTML being written, and
	//	if found, it executes those endings.
	var tryEndings = func(target uint16) {

		for htmlIndex < mostRecentEnding.index && mostRecentEnding.index < target {
			r.writer.Write(c.html[htmlIndex:mostRecentEnding.index])

			mostRecentEnding.ending()

			htmlIndex = mostRecentEnding.index

			var last = len(newlyReceivedEndings) - 1
			if last == -1 {
				mostRecentEnding = wrapperEndingAndIndex{} // clear it
				break
			}

			mostRecentEnding = newlyReceivedEndings[last]
			newlyReceivedEndings = newlyReceivedEndings[0:last]
		}

		r.writer.Write(c.html[htmlIndex:target])
		htmlIndex = target

	}

	// Helper function that invokes the given callback (pre, attrs, post) of a marker. If
	//	any wrappers were inserted during the callback's execution, they're stored until
	//	their position for insertion is reached.
	var callMarkerCallback = func(cb callbackPos[T], isAttr bool) {
		if cb.callback == nil {
			return
		}

		tryEndings(cb.pos)

		cb.callback(r, data)

		if isAttr {
			return
		}

		if r.wrapperEndings == nil {
			return
		}

		if mostRecentEnding.index != 0 {
			newlyReceivedEndings = append(newlyReceivedEndings, mostRecentEnding)
		}

		mostRecentEnding = wrapperEndingAndIndex{
			ending: r.wrapperEndings,
			index:  cb.endPos,
		}

		r.wrapperEndings = nil
	}

	var markerIndex = 0
	var markerEndIndex = len(c.markers)

	if doingWrapperEnding { // We're doing the last part of a wrapper component
		markerIndex = c.wrapperContentMarkerIndex
		htmlIndex = c.wrapperContentHTMLIndex

	} else if !finishComponent {
		markerEndIndex = c.wrapperContentMarkerIndex
	}

	// Iterate the markers of the current component (or perhaps part, if it's a wrapper)
	for ; markerIndex < markerEndIndex; markerIndex++ {
		var m = c.markers[markerIndex]

		r.wrapperEndings = nil
		r.componentState.flags = 0

		callMarkerCallback(m.callbacks[preCallbackIndex], false)

		var contentMarkerWasRemoved bool

		if r.responseState.has(skipElement) {
			contentMarkerWasRemoved = m.containsWrapperContentMarker

			htmlIndex = m.callbacks[preCallbackIndex].endPos
			r.componentState.unset(skipElement)

		} else {
			callMarkerCallback(m.callbacks[attrCallbackIndex], true)

			callMarkerCallback(m.callbacks[postCallbackIndex], false)

			if r.responseState.has(skipContent) {
				contentMarkerWasRemoved = m.containsWrapperContentMarker

				htmlIndex = m.callbacks[postCallbackIndex].endPos
				r.componentState.unset(skipContent)
			}
		}

		if contentMarkerWasRemoved {
			markerEndIndex = len(c.markers)          // Now we need to traverse to the last marker
			isWrapper, finishComponent = false, true // Technically no longer a wrapper (for this execution)
		}
	}

	if finishComponent {
		tryEndings(uint16(len(c.html)))

	} else {
		tryEndings(c.wrapperContentHTMLIndex)
	}
}

type RouteResponse struct {
	r response[*RouteData]
}

// Send503 sends a `StatusServiceUnavailable` response.
func (r *RouteResponse) Send503(err error) {
	fmt.Println(err)

	if !r.r.responseState.has(sent) {
		r.r.responseState.set(sent)
		r.r.resp.WriteHeader(http.StatusServiceUnavailable)
	}
}

// Redirect sends a redirect with the given response `code` to the given `url`.
func (r *RouteResponse) Redirect(code int, url string) {
	r.doRedirect(code, url)
}

// TemporaryRedirect sends a `StatusTemporaryRedirect` (307) response.
func (r *RouteResponse) TemporaryRedirect(url string) {
	r.doRedirect(http.StatusTemporaryRedirect, url)
}

// RedirectToGet sends a `StatusSeeOther` (303) response to redirect a POST to
// a GET request.
func (r *RouteResponse) RedirectToGet(url string) {
	r.doRedirect(http.StatusSeeOther, url)
}

// PermanentRedirect sends a `StatusMovedPermanently` (301) response.
func (r *RouteResponse) PermanentRedirect(url string) {
	r.doRedirect(http.StatusMovedPermanently, url)
}

func (r *RouteResponse) doRedirect(code int, url string) {
	if r.r.responseState.has(sent) {
		return
	}
	r.r.responseState.set(sent)

	//		r.sendCookies()

	http.Redirect(r.r.resp, r.r.req, url, code)
}

/*
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
