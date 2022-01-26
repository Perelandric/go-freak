package freak

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	html_parser "golang.org/x/net/html"
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

/*
	freak.Markers{ // This is a slice

		{
			"fooMarker",
			freak.Static{},  // This is an array of 3
			freak.Positions{ // This is an array of 3
				freak.Pre(func(r *Response, d *Data) {
					// r.AddString("whatever")
				}),
				freak.Attrs(func(r *AttrResponse, d *Data) {
					r.AddAttr("the-key", "the-value")
				}),
				freak.Post(func(r *Response, d *Data) {
					// r.AddBytes(d.ByteData)
				}),
			},
		},

		{
			"barMarker",
			freak.Static{},
			freak.Positions{
				freak.Pre(func(r *Response, d *Data) {
					// r.AddStringNoEscape("whatever")
				}),
				freak.Attrs(nil),
				freak.Post(nil),
			},
		},
	}
*/

type Marker[T any] struct {
	Name      string
	Static    Static[T]
	Positions Positions[T]
}

type MarkerCallback[T any] func(*response[T], T)

type Static[T any] [3]do[T]

type Positions[T any] [3]do[T]

type do[T any] interface {
	do(*response[T], T)
}

type Pre[T any] MarkerCallback[T]

func (b Pre[T]) do(r *response[T], data T) {
	b(r, data)
}

type Attrs[T any] func(*AttrResponse[T], T)

func (a Attrs[T]) do(r *response[T], data T) {
	a(&AttrResponse[T]{r: r}, data)
}

type Post[T any] MarkerCallback[T]

func (a Post[T]) do(r *response[T], data T) {
	a(r, data)
}

const ( // For the 'callbacks' field of 'marker'
	preCallbackIndex = iota
	attrCallbackIndex
	postCallbackIndex
)

type marker[T any] struct {
	callbacks                    [3]callbackPos[T] // for pre, attrs, post
	containsWrapperContentMarker bool
}

type callbackPos[T any] struct {
	callback    MarkerCallback[T]
	pos, endPos uint16
}

type wrapperEndingAndIndex struct {
	ending func()
	index  uint16
}

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

type HeadMarker[T any] struct {
	Static  string
	Dynamic MarkerCallback[T]
}

type headMarker[T any] struct {
	name    string
	dynamic MarkerCallback[T]
}

type Head[T any] struct {
	Title, Style, NoScript HeadMarker[T]
	Link, Script, Template []HeadMarker[T]
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

func (m *Meta) String() string {

	var b = strings.Builder{}

	b.WriteString(`<meta charset="UTF-8">`)

	if m.ApplicationName != "" {
		fmt.Fprintf(&b, `<meta name=ApplicationName content=%q>`, m.ApplicationName)
	}
	if m.Author != "" {
		fmt.Fprintf(&b, `<meta name=Author content=%q>`, m.Author)
	}
	if m.Description != "" {
		fmt.Fprintf(&b, `<meta name=Description content=%q>`, m.Description)
	}
	if m.Generator != "" {
		fmt.Fprintf(&b, `<meta name=Generator content=%q>`, m.Generator)
	}
	if m.ThemeColor != "" {
		fmt.Fprintf(&b, `<meta name=ThemeColor content=%q>`, m.ThemeColor)
	}
	if m.ColorScheme != "" {
		fmt.Fprintf(&b, `<meta name=ColorScheme content=%q>`, m.ColorScheme)
	}
	if len(m.Keywords) != 0 {
		fmt.Fprintf(&b, `<meta name=Keywords content=%q>`, strings.Join(m.Keywords, ","))
	}
	if m.Referrer != SkipReferrer {
		fmt.Fprintf(&b, `<meta name=Referrer content=%q>`, m.Referrer.String())
	}

	return b.String()
}

const (
	SkipReferrer = 0
	NoReferrer   = Referrer(1 << iota)
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

func NewPage[T any](
	h Head[T],
	bodyAttrs map[string]string,
	body func(r *RouteResponse, data T),
) Page[T] {

	var headMarkers = []*headMarker[T]{}
	var html strings.Builder

	var addTag = func(
		name string,
		attrs map[string]string,
		content *HeadMarker[T],
		doCloser bool,
		skipIfEmpty bool,
	) {

		if skipIfEmpty && len(attrs) == 0 && (content == nil || (len(content.Static) == 0 && content.Dynamic == nil)) {
			return
		}

		html.WriteByte('<')
		html.WriteString(name)
		for key, val := range attrs {
			writeAttr[T](&html, key, val, true)
		}
		html.WriteByte('>')

		html.WriteString(content.Static)

		if content.Dynamic != nil {
			// TODO: Add Marker
		}

		if doCloser {
			html.WriteString("</")
			html.WriteString(name)
			html.WriteByte('>')
		}
	}

	html.WriteString(`<!doctype html><html><head>`)

	addTag("title", nil, &h.Title, true, false)

	html.WriteString(`<meta charset="UTF-8">`)
	html.WriteString(h.Meta.String())

	addTag("style", nil, &h.Style, true, true)

	for _, m := range h.Link {
		addTag("link", map[string]string{"rel": "stylesheet", "href": m.Static}, nil, false, true)
	}

	// For the accumulated CSS. The server responds directly with this.
	addTag("link", map[string]string{"rel": "stylesheet", "href": _cssInsertionPath}, nil, false, true)

	for _, m := range h.Script {
		addTag("script", map[string]string{"src": m.Static}, nil, true, true)
	}

	// For the accumulated JS. The server responds directly with this
	addTag("script", map[string]string{"src": _jsInsertionPath}, nil, true, true)

	addTag("noscript", nil, &h.NoScript, true, true)

	for _, m := range h.Template {
		addTag("template", nil, &m, false, true)
	}

	html.WriteString("</head>")

	addTag("body", bodyAttrs, nil, true, false)

	html.WriteString("</html>")

	return Page[T]{
		pageComponent: &pageComponent[T]{
			page_head_markers: headMarkers,
			body:              body,
		},
	}
}

type pageComponent[T any] struct {
	page_head_markers []*headMarker[T]
	body              func(*RouteResponse, T)
}

type Page[T any] struct {
	*pageComponent[T]
}

func (p *Page[T]) serve(r *RouteResponse, data T) {
	// TODO: serve the page
	p.body(r, data)
}

type component[T any] struct {
	html    []byte
	compId  string
	markers []*marker[T]

	wrapperContentMarkerIndex int
	wrapperContentHTMLIndex   uint16

	compressionLevel htmlFlagHolder
}

type Component[T any] struct {
	*component[T]
}

func (p *Component[T]) serve(r *RouteResponse, data T) {
	// TODO: serve the component
}

func NewComponent[T any](css css, js js, html *html, markers ...Marker[T]) Component[T] {
	var c = component[T]{
		compId:                    nextId(),
		wrapperContentMarkerIndex: -1,
	}
	html.compId = c.compId
	addToCssJs(c.compId, css, js)
	c.processHTML(html.in, html.level, markers)

	return Component[T]{component: &c}
}

func (c *component[T]) render(
	root *html_parser.Node, buf *bytes.Buffer, isTop bool, markers []Marker[T],
) {
	var set_marker_pos = func(newMarker *marker[T], callbackIndex int, isStartPos bool) {
		if newMarker == nil {
			return
		}

		if isStartPos {
			newMarker.callbacks[callbackIndex].pos = uint16(buf.Len())
		} else {
			newMarker.callbacks[callbackIndex].endPos = uint16(buf.Len())
		}
	}

	for currNode := root; currNode != nil; currNode = currNode.NextSibling {
		switch currNode.Type {

		default:
			html_parser.Render(buf, currNode)

		case html_parser.CommentNode:
			if strings.TrimSpace(currNode.Data) == "freak-wrapped-content" {
				if c.wrapperContentMarkerIndex != -1 {
					panic(`only one "freak-wrapped-content" is permitted in a component`)
				}

				c.wrapperContentMarkerIndex = len(c.markers)
				c.wrapperContentHTMLIndex = uint16(buf.Len())

				for _, m := range c.markers {
					if m.callbacks[preCallbackIndex].endPos == 0 {
						// If the content marker is inside other markers, their 'endPos' will still be '0'
						m.containsWrapperContentMarker = true
					}
				}
			}

			continue

		case html_parser.ErrorNode, html_parser.RawNode:
			panic(currNode.Data)

		case html_parser.DocumentNode:
			// We want to traverse its children (probably !doctype and html)
			c.render(currNode.FirstChild, buf, false, markers)
			return

		case html_parser.ElementNode:
			var newMarker *marker[T]
			newMarker, markers = c.processFreakAttr(currNode, isTop, markers)

			/*
				if c.canElideOpener(currNode) {

					// If whitespace compression is enabled and
					// 	the previous sibling ends in space, and
					//	the first child of current element starts with space,
					//	eliminate the leading space in the first child node (since
					//	the previous sibling has already been rendered)

					if c.compressionLevel.hasAny(compressWhitespace|compressWhitespaceExtreme) &&
						lastCharIsSpace(currNode.PrevSibling) &&
						firstCharIsSpace(currNode.FirstChild) {
						currNode.FirstChild.Data = currNode.FirstChild.Data[1:]

						if currNode.FirstChild.Data == "" {
							removeNode(currNode.FirstChild)
						}
					}

				} else {
			*/

			set_marker_pos(newMarker, preCallbackIndex, true)

			buf.WriteByte('<')
			buf.WriteString(currNode.Data)

			var needSpace = true
			for i, attr := range sortAttrs(currNode.Attr) {
				if i == 0 ||
					needSpace ||
					c.compressionLevel.hasNone(compressWhitespaceExtreme) {
					buf.WriteByte(' ')
				}

				buf.WriteString(attr.Key)
				buf.WriteByte('=')

				var quotedVal, wasQuoted = c.quoteAttr(attr.Val)

				buf.WriteString(quotedVal)

				needSpace = !wasQuoted

				// TODO: Eventually compress proper boolean attr values to nothing.
				// 	`disabled="disabled"` or `disabled=""` becomes `disabled`
				// 		It would only be done on specific attrs for specific elems.
			}

			set_marker_pos(newMarker, attrCallbackIndex, true)
			set_marker_pos(newMarker, attrCallbackIndex, false)

			buf.WriteByte('>')

			set_marker_pos(newMarker, postCallbackIndex, true)
			// }

			c.render(currNode.FirstChild, buf, false, markers)

			set_marker_pos(newMarker, postCallbackIndex, false)

			if c.canElideCloser(currNode) {

				// If whitespace compression is enabled and
				// 	the last child of current element ends in space, and
				//	the next sibling of current element starts with space,
				//	eliminate the leading space in the next node (since the
				//	last child has already been rendered)

				if c.compressionLevel.hasAny(compressWhitespace|compressWhitespaceExtreme) &&
					lastCharIsSpace(currNode.LastChild) &&
					firstCharIsSpace(currNode.NextSibling) {
					currNode.NextSibling.Data = currNode.NextSibling.Data[1:]

					if currNode.NextSibling.Data == "" {
						removeNode(currNode.NextSibling)
					}
				}

			} else {
				fmt.Fprintf(buf, "</%s>", currNode.Data)
			}

			set_marker_pos(newMarker, preCallbackIndex, false)
		}
	}

	return
}

// If 'data-freak' attribute is found, it check that its name is the same as in the next Marker provided
// by the user. It also creates a new *marker for each Marker/position combination, and adds them to the
// component[T]. The user-defined Marker is removed from the original slice. Each added *marker is also
// returned in its own "position" based slice, so that the proper ending position can be added to each.
func (c *component[T]) processFreakAttr(
	node *html_parser.Node, isTop bool, userMarkers []Marker[T],
) (*marker[T], []Marker[T]) {

	/*
		User provides:
			data-freak="markerName" data-freak-js="Foo" (optional) data-freak-wrapper="" (optional)

		Code generates:
			data-freak="compID"
			data-freak="compID;markerName" data-freak-js="Foo"

			//          v--- leading colon means it's a nested marker
			data-freak=":compID;markerName"
	*/

	var newMarker *marker[T]

	var foundJS = false

	for i := range node.Attr {

		var attr = &node.Attr[i]

		// normalize attr key to lower case
		attr.Key = strings.ToLower(attr.Key)

		if attr.Key == dataFreakJSAttr {
			foundJS = true
			continue
		}

		if attr.Key == dataFreakAttr {
			if len(userMarkers) == 0 || userMarkers[0].Name != attr.Val {
				panic(fmt.Sprintf("No marker callbacks found for the %q marker", attr.Val))
			}

			var uM = userMarkers[0]
			userMarkers = userMarkers[1:]

			newMarker = &marker[T]{}
			c.markers = append(c.markers, newMarker)

			var foundPre, foundAttrs, foundPost bool

			for _, pos := range uM.Positions {
				var cb = callbackPos[T]{
					callback: pos.do,
				}

				switch pos.(type) {

				case Pre[T]:
					if foundPre || foundAttrs || foundPost {
						goto OUT_OF_ORDER
					}
					foundPre = true
					newMarker.callbacks[preCallbackIndex] = cb

				case Attrs[T]:
					if foundAttrs || foundPost {
						goto OUT_OF_ORDER
					}
					foundAttrs = true
					newMarker.callbacks[attrCallbackIndex] = cb

				case Post[T]:
					if foundPost {
						goto OUT_OF_ORDER
					}
					foundPost = true
					newMarker.callbacks[postCallbackIndex] = cb

				default:
					panic("unreachable")
				}

				continue

			OUT_OF_ORDER:
				panic(fmt.Sprintf("Positions for marker %q are out of order", uM.Name))
			}

			if isTop {
				attr.Val = c.compId + ";" + attr.Val
			} else {
				attr.Val = ":" + c.compId + ";" + attr.Val
			}

			return newMarker, userMarkers
		}
	}

	if isTop { // ...if no marker at the top, the comp id is used
		node.Attr = append(node.Attr, html_parser.Attribute{
			Key: dataFreakAttr,
			Val: c.compId,
		})
	} else if foundJS { // ...if no marker, but JS was found, the comp id is used
		node.Attr = append(node.Attr, html_parser.Attribute{
			Key: dataFreakJSAttr,
			Val: ":" + c.compId,
		})
	}

	return nil, userMarkers
}

func sortAttrs(attrs []html_parser.Attribute) []html_parser.Attribute {
	sort.Slice(attrs, func(i, j int) bool {
		var attrI = attrs[i].Key
		var attrJ = attrs[j].Key

		var iIsData = strings.HasPrefix(attrI, "data-")
		var jIsData = strings.HasPrefix(attrJ, "data-")

		if iIsData != jIsData {
			return jIsData
		}

		return strings.Compare(attrI, attrJ) < 0
	})

	return attrs
}

func (c *component[T]) quoteAttr(val string) (string, bool) {
	if len(val) == 0 || strings.ContainsAny(val, "\"\r\n\t ") {
		return strconv.Quote(val), true
	}
	return val, false
}
