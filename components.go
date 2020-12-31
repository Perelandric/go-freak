package freak

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"sync"
	"sync/atomic"
)

var allCss, allJs bytes.Buffer
var cssMux, jsMux sync.Mutex

func addToCss(id uint32, css string) {
	var freakId = fmt.Sprintf(`[data-freak-id="%s%d"]`, freakPrefix, id)
	css = strings.ReplaceAll(css, ":root", freakId)

	cssMux.Lock()
	defer cssMux.Unlock()
	allCss.WriteString(css)
}
func addToJs(id uint32, js string) {
	jsMux.Lock()
	defer jsMux.Unlock()
	allJs.WriteString(js)
}

const freakPrefix = "freak_"

var freakId uint32 = 0

func nextId() uint32 {
	return atomic.AddUint32(&freakId, 1)
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
	compId            uint32
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
	addToCss(c.compId, css.css)
	addToJs(c.compId, js.js)
	processFuncs(html.out, markers, &c, nil)
	return &c
}

type wrapper struct {
	compId      uint32
	preContent  component
	postContent component
}

func Wrapper(css css, js js, html *html, markers ...Marker) *wrapper {
	var c component
	var w = wrapper{
		compId: nextId(),
	}
	addToCss(c.compId, css.css)
	addToJs(w.compId, js.js)
	processFuncs(html.out, markers, &c, &w)
	return &w
}
