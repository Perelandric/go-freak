package freak

import (
	"io"
	"io/fs"
	"strings"
)

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
	markers           []*marker
	htmlTail          []byte
	maxWrapperNesting int
}

type route component

func NewRoute(css css, js js, html *html, markers ...Marker) *route {
	return (*route)(Component(css, js, html, markers...))
}

func Component(css css, js js, html *html, markers ...Marker) *component {
	var c component
	processFuncs(css.css, js.js, html.out, markers, &c, nil)
	return &c
}

type wrapper struct {
	preContent  component
	postContent component
}

func Wrapper(css css, js js, html *html, markers ...Marker) *wrapper {
	var c component
	var w wrapper
	processFuncs(css.css, js.js, html.out, markers, &c, &w)
	return &w
}
