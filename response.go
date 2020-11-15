package freak

import (
	"net/http"
)

type RouteData struct {
}

type Response struct {
	resp http.ResponseWriter
	req  *http.Request

	wrapperEndingFuncs []func(*Response)

	halt bool

	buf []byte
}

func (c *component) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var resp = Response{
		req:  r,
		resp: w,
		buf:  make([]byte, 0, 512),
	}

	c.do(&resp, &RouteData{})

	if resp.halt {
		return
	}

	w.Write(resp.buf)
}

func (r *Response) WriteBytes(b []byte) {
	r.buf = append(r.buf, escapeHTMLBytes(b)...)
}

func (r *Response) WriteString(s string) {
	r.buf = append(r.buf, escapeHTMLString(s)...)
}

func (r *Response) WriteStringNoEscape(s string) {
	r.buf = append(r.buf, s...)
}

func (r *Response) WriteBytesNoEscape(b []byte) {
	r.buf = append(r.buf, b...)
}

func (r *Response) LoadComponent(c *component, dataI interface{}) {
	c.do(r, dataI)
}

type WrapperResponse struct {
	r *Response
}

func (wr *WrapperResponse) LoadWrapper(w *wrapper, dataI interface{}) {
	if w == nil {
		return
	}

	// Originally provided by the calling component
	var temp = wr.r.wrapperEndingFuncs
	wr.r.wrapperEndingFuncs = nil

	w.preContent.do(wr.r, dataI)

	wr.r.wrapperEndingFuncs = append(temp, func(r *Response) {
		w.postContent.do(r, dataI)
	})
}
