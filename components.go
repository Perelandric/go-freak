package freak

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

const freakPrefix = "freak_"

var freakId uint32 = 0

func nextId() string {
	return strconv.FormatUint(uint64(atomic.AddUint32(&freakId, 1)), 16)
}

var allCss, allJs bytes.Buffer
var cssMux, jsMux sync.Mutex

const _resDir = "/res/"

const _cssInsertionPath = _resDir + "freak-css.css"
const _jsInsertionPath = _resDir + "freak-js.js"

func addToCss(id string, css string) {
	if len(css) == 0 {
		return
	}

	var freakDataAttr = fmt.Sprintf(`[data-freak-id="%s%s"]`, freakPrefix, id)
	css = strings.ReplaceAll(css, ":root", freakDataAttr)

	cssMux.Lock()
	defer cssMux.Unlock()

	allCss.WriteString(css)
}
func addToJs(id string, js string) {
	if len(js) == 0 {
		return
	}

	var newJS = strings.Replace(js, "export default", "return ", 1)
	newJS = fmt.Sprintf(";freak.%s=((freak)=>{%s}(freak));", id, newJS)

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

func HTML(s string) *html {
	return &html{in: s, out: s}
}

func HTMLFile(f fs.File) *html {
	return HTML(fileToString(f))
}

type StringFunc struct {
	Static  string
	Dynamic func(*Response, *RouteData)
}
type Page struct {
	Head      Head
	BodyAttrs map[string]string
	Body      StringFunc
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

func (p *Page) build() *component {
	var markers = []Marker{}
	var html strings.Builder
	html.WriteString(`<!doctype html><title>`)

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

	addStringOrFunc("", p.Head.Title, "")

	html.WriteString(`</title><meta charset="UTF-8">`)

	var mVal = reflect.ValueOf(p.Head.Meta)
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

	addStringOrFunc(`<style>`, p.Head.Style, `</style>`)

	for _, m := range p.Head.Link {
		addStringOrFunc(`<link rel="stylesheet" href="`, m, `">`)
	}

	// For the accumulated CSS. The server responds directly with this
	html.WriteString(`<link rel="stylesheet" href="`)
	html.WriteString(_cssInsertionPath)
	html.WriteString(`">`)

	html.WriteString(`<script>const freak={}</script>`)

	for _, m := range p.Head.Script {
		addStringOrFunc(`<script src="`, m, `"></script>`)
	}

	// For the accumulated JS. The server responds directly with this
	html.WriteString(`<script src="`)
	html.WriteString(_jsInsertionPath)
	html.WriteString(`"></script>`)

	addStringOrFunc(`<noscript>`, p.Head.NoScript, `</noscript>`)

	for _, m := range p.Head.Template {
		addStringOrFunc(`<template>`, m, `</template>`)
	}

	html.WriteString("<body")
	for k, v := range p.BodyAttrs {
		fmt.Fprintf(&html, " %s=%q", k, v)
	}
	html.WriteByte('>')

	addStringOrFunc("", p.Body, "")

	html.WriteString("</body></html>")

	return NewComponent(
		CSS(""),
		JS(""),
		HTML(html.String()).Extreme(),
		markers...,
	)
}

func NewPage(page Page, markers ...Marker) *component {
	return page.build()
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
	addToCss(c.compId, css.css)
	addToJs(c.compId, js.js)
	processFuncs(html.out, markers, &c, nil)
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
	addToCss(c.compId, css.css)
	addToJs(w.compId, js.js)
	processFuncs(html.out, markers, &c, &w)
	return &w
}
