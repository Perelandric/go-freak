package freak

import (
	"io"
	"io/fs"
	"strconv"
	"strings"
	"sync/atomic"
)

const freakPrefix = "freak_"

var freakId uint32 = 0

func nextId() string {
	return freakPrefix + strconv.FormatUint(
		uint64(atomic.AddUint32(&freakId, 1)),
		16,
	)
}

type css struct {
	css string
}
type js struct {
	js string
}

func fileToString(f fs.File) string {
	var b strings.Builder
	io.Copy(&b, f)
	return b.String()
}

func CSS(s string) css {
	return css{s}
}

func CSSFile(f fs.File) css {
	return CSS(fileToString(f))
}

func JS(s string) js {
	return js{s}
}

func JSFile(f fs.File) js {
	return JS(fileToString(f))
}

func HTML(s string) *html {
	return &html{in: s, out: s}
}

func HTMLFile(f fs.File) *html {
	return HTML(fileToString(f))
}

type component struct {
	compId            string
	markers           []*marker
	htmlTail          []byte
	maxWrapperNesting int
}

type route component

func NewRoute(css css, js js, html *html, markers ...Marker) *route {
	return (*route)(Component(css, js, html, markers...))
}

func Component(css css, js js, html *html, markers ...Marker) *component {
	var c = component{
		compId: nextId(),
	}
	processFuncs(css.css, js.js, html.out, markers, &c, nil)
	return &c
}

type wrapper struct {
	compId      string
	preContent  component
	postContent component
}

func Wrapper(css css, js js, html *html, markers ...Marker) *wrapper {
	var c component
	var w = wrapper{
		compId: nextId(),
	}
	processFuncs(css.css, js.js, html.out, markers, &c, &w)
	return &w
}
