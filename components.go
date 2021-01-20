package freak

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
)

var freakId uint32 = 0

func nextId() string {
	return fmt.Sprintf("f%x", atomic.AddUint32(&freakId, 1))
}

var allCss, allJs bytes.Buffer
var cssMux, jsMux sync.Mutex

const _resDir = "/res/"

const _cssInsertionPath = _resDir + "freak-css.css"
const _jsInsertionPath = _resDir + "freak-js.js"

func addToCssJs(id string, css css, js js) {
	if len(css.css) == 0 {
		goto doJS
	}

	css.css = strings.ReplaceAll(
		css.css,
		":root",
		fmt.Sprintf(`[data-freak^=%q]`, id+":"),
	)

	cssMux.Lock()
	defer cssMux.Unlock()

	allCss.WriteString(css.css)

doJS:
	if len(js.js) == 0 {
		return
	}

	var newJS = strings.Replace(js.js, "export default", "return ", 1)
	newJS = fmt.Sprintf("[%q,freak=>{%s}],", id, newJS)

	jsMux.Lock()
	defer jsMux.Unlock()

	allJs.WriteString(newJS)
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

func HTML(s string, compress HTMLCompress) *html {
	return &html{
		in:    s,
		out:   s,
		level: htmlFlagHolder{_no_touchy: compress},
	}
}

func HTMLFile(f fs.File, compress HTMLCompress) *html {
	return HTML(fileToString(f), compress)
}

type StringFunc struct {
	Static  string
	Dynamic func(*Response, *RouteData)
}
type Head struct {
	Title, Style, NoScript StringFunc
	Link, Script, Template []StringFunc
	Meta                   Meta
}
type Meta struct {
	ApplicationName string
	Author          string
	Description     string
	Generator       string
	Keywords        []string
	Referrer        Referrer
	ThemeColor      string
	ColorScheme     string
}
type Referrer uint

const (
	NoReferrer = Referrer(1 << iota)
	Origin
	NoReferrerWhenDowngrade
	OriginWhenCrossOrigin
	SameOrigin
	StrictOrigin
	StritOriginWhenCrossOrigin
	UnsafeURL
)

func (r Referrer) String() string {
	switch r {
	case NoReferrer:
		return "no-referrer"
	case Origin:
		return "origin"
	case NoReferrerWhenDowngrade:
		return "no-referrer-when-downgrade"
	case OriginWhenCrossOrigin:
		return "origin-when-cross-origin"
	case SameOrigin:
		return "same-origin"
	case StrictOrigin:
		return "strict-origin"
	case StritOriginWhenCrossOrigin:
		return "strict-origin-when-cross-origin"
	case UnsafeURL:
		return "unsafe-url"
	default:
		return ""
	}
}

func NewPage(h Head, attrs map[string]string, content ...StringFunc) *component {
	var markers = []Marker{}
	var html strings.Builder
	var addStringOrFunc = func(pre string, sf StringFunc, post string) {
		if sf.Static == "" && sf.Dynamic == nil {
			return
		}

		html.WriteString(pre)
		html.WriteString(sf.Static)

		if sf.Dynamic != nil {
			var markerName = fmt.Sprintf("m%d", len(markers))

			fmt.Fprintf(&html, "${%s}", markerName)

			markers = append(markers, Marker{
				Name:    markerName,
				Dynamic: sf.Dynamic,
			})
		}

		html.WriteString(post)
	}

	html.WriteString(`<!doctype html><title>`)
	addStringOrFunc("", h.Title, "")
	html.WriteString(`</title><meta charset="UTF-8">`)

	var mVal = reflect.ValueOf(h.Meta)
	for i, ln := 0, mVal.NumField(); i < ln; i++ {
		var fVal = mVal.Field(i)
		var name = fVal.Type().Name()
		var content = ""

		switch v := fVal.Interface().(type) {
		case nil:
			continue
		case Referrer:
			if v != 0 {
				content = v.String()
			}
		case string:
			content = v
		case []string:
			content = strings.Join(v, ",")
		case fmt.Stringer:
			content = v.String()
		default:
			panic("unreachable")
		}

		if content != "" {
			fmt.Fprintf(&html, `<meta name=%q content=%q>`, name, content)
		}
	}

	addStringOrFunc(`<style>`, h.Style, `</style>`)

	for _, m := range h.Link {
		addStringOrFunc(`<link rel="stylesheet" href="`, m, `">`)
	}

	// For the accumulated CSS. The server responds directly with this
	fmt.Fprintf(&html, `<link rel="stylesheet" href=%q>`, _cssInsertionPath)

	for _, m := range h.Script {
		addStringOrFunc(`<script src="`, m, `"></script>`)
	}

	// For the accumulated JS. The server responds directly with this
	fmt.Fprintf(&html, `<script src=%q></script>`, _jsInsertionPath)

	addStringOrFunc(`<noscript>`, h.NoScript, `</noscript>`)

	for _, m := range h.Template {
		addStringOrFunc(`<template>`, m, `</template>`)
	}

	html.WriteString("<body")
	for k, v := range attrs {
		fmt.Fprintf(&html, " %s=%q", k, v)
	}
	html.WriteByte('>')

	for _, c := range content {
		addStringOrFunc("", c, "")
	}

	html.WriteString("</body></html>")

	return NewComponent(
		CSS(""),
		JS(""),
		HTML(html.String(), Extreme),
		markers...,
	)
}

type component struct {
	compId            string
	markers           []*marker
	htmlTail          []byte
	maxWrapperNesting int
}

func NewComponent(css css, js js, html *html, markers ...Marker) *component {
	var c = component{
		compId: nextId(),
	}
	html.compId = c.compId
	addToCssJs(c.compId, css, js)
	processFuncs(html, markers, &c, nil)
	return &c
}

type wrapper struct {
	compId      string
	preContent  component
	postContent component
}

func NewWrapper(css css, js js, html *html, markers ...Marker) *wrapper {
	var c component
	var w = wrapper{
		compId: nextId(),
	}
	html.compId = w.compId
	addToCssJs(w.compId, css, js)
	processFuncs(html, markers, &c, &w)
	return &w
}
