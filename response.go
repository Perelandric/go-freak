package freak

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"reflect"
)

type state struct {
	flags stateFlag
}
type stateFlag uint16

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

type Response struct {
	resp http.ResponseWriter
	req  *http.Request

	wrapperEndingFuncs []func(*Response)

	// This is for calling the provided callbacks via reflection.
	// It holds a circular reference to itself.
	thisAsValue reflect.Value

	halt bool

	// vvv--- from "smash"

	cookiesToSend []*http.Cookie

	state state

	gzip gzip.Writer

	// Unable to write directly to the ResponseWriter (or via gzip) because
	// that cause WriteHeader to take place with StatusOK, which means we can
	// no longer redirect.
	// So instead we must write to a bytes.Buffer.
	buf bytes.Buffer

	writer io.Writer

	siteMapNode *SiteMapNode // for the requested page
}

func getResponse(
	s *server,
	resp http.ResponseWriter,
	req *http.Request,
	node *SiteMapNode,
	doGzip bool,
) (r *Response) {

	if _poolEnabled {
		select {
		case r = <-respPool:
			goto INITIALIZE
		default:
			goto CREATE
		}
	} else {
		goto CREATE
	}

CREATE:
	{
		gz, _ := gzip.NewWriterLevel(nil, s.compressionLevel)

		r = &Response{
			gzip: *gz,
			buf:  *bytes.NewBuffer(make([]byte, 0, _bufMaxSize)),
		}
		r.thisAsValue = reflect.ValueOf(r)
	}

INITIALIZE:
	r.req = req
	r.resp = resp

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
func putResponse(s *server, r *Response) {
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
	r.halt = false

	if _poolEnabled {
		select {
		case respPool <- r: // Successfully placed back into pool

		default: // let overflow get GC'd
		}
	}
}

func (r *Response) WriteBytes(b []byte) {
	writeEscapeHTMLBytes(&r.buf, b)
}

func (r *Response) WriteString(s string) {
	writeEscapeHTMLString(&r.buf, s)
}

func (r *Response) WriteStringNoEscape(s string) {
	r.buf.WriteString(s)
}

func (r *Response) WriteBytesNoEscape(b []byte) {
	r.buf.Write(b)
}

func (r *Response) DoComponent(c *component, dataI interface{}) {
	r.do(c, dataI)
}

type WrapperResponse struct {
	r *Response
}

func (wr *WrapperResponse) DoWrapper(w *wrapper, dataI interface{}) {
	var r = wr.r

	if w == nil || r.halt {
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

func (r *Response) do(c *component, dataI interface{}) {
	if r.halt {
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

		r.buf.Write(m.htmlPrefix)

		switch m.kind {
		case plainMarker:
			m.fn.Call(callArgs[:])
			if r.halt {
				return
			}

		case wrapperStartMarker:
			endStackIndex++

			if endStackIndex >= len(wrapperEndStack) {
				panic("unreachable")
			}

			var funcSlice = wrapperEndStack[endStackIndex]
			r.wrapperEndingFuncs = funcSlice[0:0:cap(funcSlice)]

			m.fn.Call(callArgs[:])

			if r.halt {
				return
			}

			// In case the end-funcs slice was grown beyond itw original capacity
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

			for i := len(funcSlice) - 1; i != -1 && !r.halt; i-- {
				funcSlice[i](r)
				if r.halt {
					return
				}
			}

			endStackIndex--

		default:
			panic("unreachable")
		}
	}

	if endStackIndex != -1 {
		panic("unreachable")
	}

	r.buf.Write(c.htmlTail)
}
