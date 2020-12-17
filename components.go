package freak

type css struct {
	css string
}
type js struct {
	js string
}

func CSS(s string) css {
	return css{s}
}

func JS(s string) js {
	return js{s}
}

func HTML(s string) *html {
	return &html{in: s, out: s}
}

type component struct {
	markers           []*marker
	htmlTail          []byte
	maxWrapperNesting int
}

func Component(css css, js js, html html, markers ...Marker) *component {
	var c component
	processFuncs(css.css, js.js, html.out, markers, &c, nil)
	return &c
}

type wrapper struct {
	preContent  component
	postContent component
}

func Wrapper(css, js string, html html, markers ...Marker) *wrapper {
	var c component
	var w wrapper
	processFuncs(css, js, html.out, markers, &c, &w)
	return &w
}
