package freak

import (
	"bytes"
	"regexp"
	"strings"

	html_parser "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type HTMLCompress uint8

type htmlFlagHolder struct {
	_no_touchy HTMLCompress
}

func (hcf htmlFlagHolder) hasAny(f HTMLCompress) bool {
	return hcf._no_touchy&f != 0
}
func (hcf htmlFlagHolder) hasAll(f HTMLCompress) bool {
	return hcf._no_touchy&f == f
}
func (hcf htmlFlagHolder) hasNone(f HTMLCompress) bool {
	return hcf._no_touchy&f == 0
}
func (hcf htmlFlagHolder) isZero() bool {
	return hcf._no_touchy == 0
}

type html struct {
	in, out string
	compId  string
	level   htmlFlagHolder
}

const dataFreakAttr = "data-freak"
const dataFreakJSAttr = "data-freak-js"

const (
	compressAttrQuotes = HTMLCompress(1 << iota)
	compressComments
	compressEndTags
	compressStartTags
	compressWhitespace
	compressWhitespaceExtreme
)

const (
	None       = HTMLCompress(0)
	Moderate   = compressComments | compressWhitespace
	Aggressive = compressComments | compressWhitespace | compressAttrQuotes | compressEndTags
	Extreme    = compressComments | compressWhitespace | compressAttrQuotes | compressEndTags | compressStartTags | compressWhitespaceExtreme
)

func (c *component[T]) processHTML(htmlIn string, compressionLevel htmlFlagHolder, markers []Marker[T]) {
	if len(c.html) != 0 {
		return
	}
	c.html = []byte(htmlIn)

	var ctxNode = getContext(strToBytes(htmlIn))
	var node *html_parser.Node
	var nodes []*html_parser.Node
	var err error

	if ctxNode == nil { // We're at the top of a page
		node, err = html_parser.Parse(strings.NewReader(htmlIn))

	} else {
		nodes, err = html_parser.ParseFragment(strings.NewReader(htmlIn), ctxNode)

		// ParseFragment does not join as siblings, so join them
		var prev *html_parser.Node
		for _, n := range nodes {
			if prev == nil {
				node = n
			} else {
				n.PrevSibling = prev
				prev.NextSibling = n
			}
			prev = n
		}
	}

	if err != nil {
		panic(err)
	}

	// If comments are to be removed, we do it first so that newly adjacent text
	// nodes can be joined together, making space removal more accurante
	if c.compressionLevel.hasAny(compressComments) {
		node = removeComments(node)
	}

	// If whitespace is to be compressed, we do it first since it may impact tag omission
	if c.compressionLevel.hasAny(compressWhitespace | compressWhitespaceExtreme) {
		compressSpace(node, c.compressionLevel.hasAny(compressWhitespaceExtreme))
	}

	var buf bytes.Buffer
	c.render(node, &buf, true, markers)

	c.html = buf.Bytes()
}

var reTag = regexp.MustCompile(`(?i)<(!--|!doctype|[a-z][a-z0-9]*)`)

func getContext(htm []byte) *html_parser.Node {
	var idcs = reTag.FindIndex(htm)
	var start = 0
	var name string

	for idcs != nil {
		name = string(bytes.ToLower(htm[start+idcs[0]+1 : start+idcs[1]]))

		if name == "!doctype" {
			return nil // doctype decl, so it's a root
		}

		if name == "!--" {
			// HTML comment
			var closer = bytes.Index(htm[start+idcs[1]:], []byte("-->"))
			if closer == -1 { // Comment has no closer, so process as 'div'
				break
			}
			start = start + closer + 3
			idcs = reTag.FindIndex(htm[start:])
			continue // Continue after the comment
		}

		break
	}

	var a = atom.Div // Default context

	if len(name) != 0 {
		switch atom.Lookup([]byte(name)) {
		case atom.Html:
			return nil // html tag, so it's a root

		case atom.Head,
			atom.Body:
			a = atom.Html

		case atom.Title:
			a = atom.Head

		case atom.Td,
			atom.Th:
			a = atom.Tr

		case atom.Tr:
			a = atom.Tbody

		case atom.Tbody,
			atom.Thead,
			atom.Tfoot:
			a = atom.Table

		case atom.Li:
			a = atom.Ul

		case atom.Option,
			atom.Optgroup:
			a = atom.Select

		case atom.Col:
			a = atom.Colgroup
		}
	}

	return &html_parser.Node{
		Type:     html_parser.ElementNode,
		DataAtom: a,
		Data:     a.String(),
	}
}

func (c *component[T]) canElideOpener(n *html_parser.Node) bool {
	if c.compressionLevel.hasNone(compressStartTags) || len(n.Attr) != 0 {
		return false
	}

	switch n.DataAtom {
	case atom.Html:
		// An HTML element's start tag may be omitted if the first thing inside the
		// HTML element is not a comment.
		return c.compressionLevel.hasAny(compressComments) || n.FirstChild.Type != html_parser.CommentNode

	case atom.Head:
		// A HEAD element's start tag may be omitted if the element is empty, or
		// if the first thing inside the
		return n.FirstChild == nil || n.FirstChild.Type == html_parser.ElementNode

	case atom.Body:
		// A body element's start tag may be omitted if the element is empty, or
		// if the first thing inside the body element is not a space character or
		// a comment, except if the first thing inside the body element is a
		// script or style element.
		return n.FirstChild == nil ||
			(n.FirstChild.Type != html_parser.CommentNode &&
				!firstCharIsSpace(n.FirstChild) &&
				!nodeIsOneOf(n.FirstChild, atom.Script, atom.Style))

	case atom.Colgroup:
		// A colgroup element's start tag may be omitted if the first thing inside the
		// colgroup element is a col element, and if the element is not immediately
		// preceded by another colgroup element whose end tag has been omitted. (It
		// can't be omitted if the element is empty.)

		return nodeIsOneOf(n.FirstChild, atom.Col) &&
			!(nodeIsOneOf(n.PrevSibling, atom.Colgroup) && c.canElideCloser(n.PrevSibling))

	case atom.Tbody:
		// A tbody element's start tag may be omitted if the first thing inside the
		// tbody element is a tr element, and if the element is not immediately preceded
		// by a tbody, thead, or tfoot element whose end tag has been omitted. (It can't
		// be omitted if the element is empty.)

		return nodeIsOneOf(n.FirstChild, atom.Tr) &&
			!(nodeIsOneOf(n.PrevSibling, atom.Tbody, atom.Thead, atom.Tfoot) && c.canElideCloser(n.PrevSibling))

	default:
		return false
	}
}

func (c *component[T]) canElideCloser(n *html_parser.Node) bool {
	if c.compressionLevel.hasNone(compressEndTags) {
		return false
	}

	if isEmptyElement(n.DataAtom) {
		return true // elements that don't allow children never get the closer
	}

	var next = n.NextSibling

	switch n.DataAtom {
	case atom.Html:
		// An html element's end tag may be omitted if the html element is not
		// immediately followed by a comment.
		return next == nil || next.Type != html_parser.CommentNode

	case atom.Head:
		// A head element's end tag may be omitted if the head element is not
		// immediately followed by a space character or a comment.
		//
		// The not `FORM` test is for the sake of IE9 and lower
		return !nodeIsOneOf(next, atom.Form) && !firstCharIsSpace(next) // With compression, comments are removed

	case atom.Body:
		// A body element's end tag may be omitted if the body element is not
		// immediately followed by a comment.
		return next == nil || next.Type != html_parser.CommentNode

	case atom.Li:
		// An li element's end tag may be omitted if the li element is immediately
		// followed by another li element or if there is no more content in the parent
		// element.
		return next == nil || nodeIsOneOf(next, atom.Li)

	case atom.Dt:
		// A dt element's end tag may be omitted if the dt element is immediately
		// followed by another dt element or a dd element.
		return nodeIsOneOf(next, atom.Dt, atom.Dd)

	case atom.Dd:
		// A dd element's end tag may be omitted if the dd element is immediately
		// followed by another dd element or a dt element, or if there is no more
		// content in the parent element.
		return next == nil || nodeIsOneOf(next, atom.Dd, atom.Dt)

	case atom.P:
		// A p element's end tag may be omitted if the p element is immediately followed
		// by an address, article, aside, blockquote, dir, div, dl, fieldset, footer,
		// form, h1, h2, h3, h4, h5, h6, header, hgroup, hr, menu, nav, ol, p, pre,
		// section, table, or ul, element, or if there is no more content in the parent
		// element and the parent element is not an a element.

		return false // Don't bother, since valid, dynamic content after the end can cause failure.

		//return (next == nil && !isOneOf(n.Parent, atom.A)) ||
		//	isOneOf(next,
		//		atom.Address, atom.Article, atom.Aside, atom.Blockquote, atom.Dir,
		//		atom.Div, atom.Dl, atom.Fieldset, atom.Footer,
		//		// atom.Form (ie has trouble) ,
		//		atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6, atom.Header,
		//		atom.Hgroup, atom.Hr, atom.Menu, atom.Nav, atom.Ol, atom.P, atom.Pre,
		//		atom.Section, atom.Table, atom.Ul)

	case atom.Rt:
		// An rt element's end tag may be omitted if the rt element is immediately
		// followed by an rt or rp element, or if there is no more content in the parent
		// element.
		return next == nil || nodeIsOneOf(next, atom.Rt, atom.Rp)

	case atom.Rp:
		// An rp element's end tag may be omitted if the rp element is immediately
		// followed by an rt or rp element, or if there is no more content in the parent
		// element.
		return next == nil || nodeIsOneOf(next, atom.Rt, atom.Rp)

	case atom.Optgroup:
		// An optgroup element's end tag may be omitted if the optgroup element is
		// immediately followed by another optgroup element, or if there is no more
		// content in the parent element.
		return next == nil || nodeIsOneOf(next, atom.Optgroup)

	case atom.Option:
		// An option element's end tag may be omitted if the option element is
		// immediately followed by another option element, or if it is immediately
		// followed by an optgroup element, or if there is no more content in the parent
		// element.
		return next == nil || nodeIsOneOf(next, atom.Option, atom.Optgroup)

	case atom.Colgroup:
		// A colgroup element's end tag may be omitted if the colgroup element is not
		// immediately followed by a space character or a comment.
		return !firstCharIsSpace(next)

	case atom.Thead:
		// A thead element's end tag may be omitted if the thead element is immediately
		// followed by a tbody or tfoot element.
		return nodeIsOneOf(next, atom.Tbody, atom.Tfoot)

	case atom.Tbody:
		// A tbody element's end tag may be omitted if the tbody element is immediately
		// followed by a tbody or tfoot element, or if there is no more content in the
		// parent element.
		return next == nil || nodeIsOneOf(next, atom.Tbody, atom.Tfoot)

	case atom.Tfoot:
		// A tfoot element's end tag may be omitted if the tfoot element is immediately
		// followed by a tbody element, or if there is no more content in the parent
		// element.
		return next == nil || nodeIsOneOf(next, atom.Tbody)

	case atom.Tr:
		// A tr element's end tag may be omitted if the tr element is immediately
		// followed by another tr element, or if there is no more content in the parent
		// element.
		return next == nil || nodeIsOneOf(next, atom.Tr)

	case atom.Td:
		// A td element's end tag may be omitted if the td element is immediately
		// followed by a td or th element, or if there is no more content in the parent
		// element.
		return next == nil || nodeIsOneOf(next, atom.Td, atom.Th)

	case atom.Th:
		// A th element's end tag may be omitted if the th element is immediately
		// followed by a td or th element, or if there is no more content in the parent
		// element.
		return next == nil || nodeIsOneOf(next, atom.Td, atom.Th)

	default:
		return false
	}
}

func isTextWithData(n *html_parser.Node) bool {
	return n != nil && n.Type == html_parser.TextNode && len(n.Data) != 0
}

func firstCharIsSpace(n *html_parser.Node) bool {
	return isTextWithData(n) && reSpaces.MatchString(n.Data[0:1])
}

func lastCharIsSpace(n *html_parser.Node) bool {
	return isTextWithData(n) && reSpaces.MatchString(n.Data[len(n.Data)-1:])
}

func nodeIsOneOf(n *html_parser.Node, atoms ...atom.Atom) bool {
	if n == nil {
		return false
	}
	for _, a := range atoms {
		if n.DataAtom == a {
			return true
		}
	}
	return false
}

func removeNode(n *html_parser.Node) (prev, next *html_parser.Node) {
	prev, next = n.PrevSibling, n.NextSibling

	if prev != nil {
		prev.NextSibling = next
		n.PrevSibling = nil
	}
	if next != nil {
		next.PrevSibling = prev
		n.NextSibling = nil
	}

	if n.Parent != nil {
		if prev == nil {
			n.Parent.FirstChild = next
		}
		if next == nil {
			n.Parent.LastChild = prev
		}
		n.Parent = nil
	}

	return prev, next
}

func removeComments(n *html_parser.Node) *html_parser.Node {
	currNode := n

	for currNode != nil {
		if currNode.Type != html_parser.CommentNode {
			removeComments(currNode.FirstChild)
			currNode = currNode.NextSibling

			continue
		}

		// Remove the comment node
		var prev, next = removeNode(currNode)

		// If prev and its new sibling are text nodes, join their text into prev
		// and remove that sibling
		for prev != nil && next != nil && prev.Type == html_parser.TextNode && next.Type == html_parser.TextNode {
			prev.Data += next.Data
			prev, next = removeNode(next)
		}

		if currNode == n {
			// We're on the first node that was passed in, so update it for the return value
			n = next
		}

		currNode = next
	}

	return n
}

var reSpaces = regexp.MustCompile(`\s+`)

func compressSpace(n *html_parser.Node, isExtreme bool) {
	currNode := n

	for currNode != nil {

		if currNode.Type != html_parser.TextNode {
			if canCompressElem(currNode.DataAtom) {
				compressSpace(currNode.FirstChild, isExtreme)
			}

			currNode = currNode.NextSibling
			continue
		}

		if isExtreme && len(strings.TrimSpace(currNode.Data)) == 0 {
			// "extreme" whitespace compression removes whitespace-only text nodes
			_, currNode = removeNode(currNode)

		} else {
			currNode.Data = reSpaces.ReplaceAllString(currNode.Data, " ")
			currNode = currNode.NextSibling
		}
	}
}

func canCompressElem(a atom.Atom) bool {
	switch a {
	case atom.Pre, atom.Script, atom.Style:
		return false
	default:
		return true
	}
}

func isEmptyElement(name atom.Atom) bool {
	switch name {
	// HTML4
	case atom.Area,
		atom.Base,
		atom.Basefont, // obsolete
		atom.Br,
		atom.Col,
		atom.Command, // obsolete
		atom.Embed,
		atom.Hr,
		atom.Img,
		atom.Input,
		atom.Isindex, // obsolete
		atom.Keygen,  // obsolete
		atom.Link,
		atom.Meta,
		atom.Param,

		// HTML5
		atom.Source,
		atom.Track,
		atom.Wbr:
		return true

	default:
		return false
	}
}
