package freak

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type HTMLCompressFlag uint8

const (
	HTMLAttrQuotes = HTMLCompressFlag(1 << iota)
	HTMLComments
	HTMLEndTags
	HTMLStartTags
	HTMLWhitespace

	HTMLCompressNone = HTMLCompressFlag(0)
	HTMLCompressAll  = HTMLCompressFlag(
		HTMLAttrQuotes | HTMLComments | HTMLEndTags | HTMLStartTags | HTMLWhitespace,
	)
)

func compressHTML(flags HTMLCompressFlag, markup HTML) string {
	if flags == HTMLCompressNone {
		return string(markup)
	}

	var ctxNode = getContext(strToBytes(string(markup)))

	var nodes, err = html.ParseFragment(strings.NewReader(string(markup)), ctxNode)
	if err != nil {
		panic(err)
	}

	var buf strings.Builder
	for _, n := range nodes {
		render(n, &buf, flags)
	}

	return buf.String()
}

var reTag = regexp.MustCompile(`<(!(--)?)?[a-zA-Z][a-zA-Z0-9]*`)

func getContext(htm []byte) *html.Node {
	var idcs = reTag.FindIndex(htm)
	var name []byte

	for idcs != nil {
		name = bytes.ToLower(htm[idcs[0]+1 : idcs[1]])

		if name[0] != '!' {
			break // Just a regluar tag name, so break the loop
		}

		if bytes.Equal(name[1:], []byte("doctype")) {
			return nil // doctype decl, so it's a root
		}

		if !bytes.HasPrefix(name[1:], []byte("--")) {
			break // Unknown '!' tag, so process as 'div'
		}

		// HTML comment
		var closer = bytes.Index(htm[idcs[1]:], []byte("-->"))
		if closer == -1 { // Comment has no closer, so process as 'div'
			break
		}
		idcs = reTag.FindIndex(htm[closer+3:]) // Continue after the comment
	}

	var a = atom.Div // Default context

	if len(name) != 0 {
		switch atom.Lookup(name) {
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

	return &html.Node{
		Type:     html.ElementNode,
		DataAtom: a,
		Data:     a.String(),
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

var reSpaces = regexp.MustCompile(`\s+`)

func canCompressElem(a atom.Atom) bool {
	switch a {
	case atom.Pre, atom.Script, atom.Style:
		return false
	default:
		return true
	}
}

func canElideOpener(n *html.Node, flags HTMLCompressFlag) bool {
	if flags&HTMLStartTags == 0 || len(n.Attr) != 0 {
		return false
	}

	switch n.DataAtom {
	case atom.Html:
		// An HTML element's start tag may be omitted if the first thing inside the
		// HTML element is not a comment.
		return true // compression removes all comments

	case atom.Head:
		// A HEAD element's start tag may be omitted if the element is empty, or
		// if the first thing inside the HEAD element is an element.
		var firstChild = compressedNode(n.FirstChild, flags)

		return firstChild == nil || firstChild.Type == html.ElementNode

	case atom.Body:
		// A body element's start tag may be omitted if the element is empty, or
		// if the first thing inside the body element is not a space character or
		// a comment, except if the first thing inside the body element is a
		// script or style element.
		return n.FirstChild == nil ||
			(n.FirstChild.Type != html.CommentNode &&
				!firstCharIsSpace(n.FirstChild) &&
				!isOneOf(n.FirstChild, atom.Script, atom.Style))

	case atom.Colgroup:
		// A colgroup element's start tag may be omitted if the first thing inside the
		// colgroup element is a col element, and if the element is not immediately
		// preceded by another colgroup element whose end tag has been omitted. (It
		// can't be omitted if the element is empty.)
		return false // TODO: Implement

	case atom.Tbody:
		// A tbody element's start tag may be omitted if the first thing inside the
		// tbody element is a tr element, and if the element is not immediately preceded
		// by a tbody, thead, or tfoot element whose end tag has been omitted. (It can't
		// be omitted if the element is empty.)
		return false // TODO: Implement

	default:
		return false
	}
}

func firstCharIsSpace(n *html.Node) bool {
	return n != nil &&
		n.Type == html.TextNode &&
		len(n.Data) != 0 &&
		reSpaces.MatchString(n.Data[0:1])
}

func isOneOf(n *html.Node, atoms ...atom.Atom) bool {
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

func compressedNode(n *html.Node, flags HTMLCompressFlag) *html.Node {
	var removeComments = flags&HTMLComments == HTMLComments
	var compressSpace = flags&HTMLWhitespace == HTMLWhitespace

	if !removeComments && !compressSpace {
		return n
	}

	for {
		if n == nil {
			return n
		}

		if removeComments && n.Type == html.CommentNode {
			n = n.NextSibling
			continue // Comment nodes will get compressed away
		}

		if compressSpace && n.Type == html.TextNode {
			var trimmed = strings.TrimSpace(n.Data)
			if len(trimmed) == 0 {
				n = n.NextSibling
				continue // Whitespace-only text nodes will get compressed away
			}
		}
		return n
	}
}

func canElideCloser(n *html.Node, flags HTMLCompressFlag) bool {
	if flags&HTMLEndTags == 0 {
		return false
	}

	if isEmptyElement(n.DataAtom) {
		return true // elements that don't allow children never get the closer
	}

	var next = compressedNode(n.NextSibling, flags)

	switch n.DataAtom {
	case atom.Html:
		// An html element's end tag may be omitted if the html element is not
		// immediately followed by a comment.
		return true // With compression, comments are removed

	case atom.Head:
		// A head element's end tag may be omitted if the head element is not
		// immediately followed by a space character or a comment.
		//
		// The not `FORM` test is for the sake of IE9 and lower
		return !isOneOf(next, atom.Form) && !firstCharIsSpace(next) // With compression, comments are removed

	case atom.Body:
		// A body element's end tag may be omitted if the body element is not
		// immediately followed by a comment.
		return true // With compression, comments are removed

	case atom.Li:
		// An li element's end tag may be omitted if the li element is immediately
		// followed by another li element or if there is no more content in the parent
		// element.
		return next == nil || isOneOf(next, atom.Li)

	case atom.Dt:
		// A dt element's end tag may be omitted if the dt element is immediately
		// followed by another dt element or a dd element.
		return isOneOf(next, atom.Dt, atom.Dd)

	case atom.Dd:
		// A dd element's end tag may be omitted if the dd element is immediately
		// followed by another dd element or a dt element, or if there is no more
		// content in the parent element.
		return next == nil || isOneOf(next, atom.Dd, atom.Dt)

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
		return next == nil || isOneOf(next, atom.Rt, atom.Rp)

	case atom.Rp:
		// An rp element's end tag may be omitted if the rp element is immediately
		// followed by an rt or rp element, or if there is no more content in the parent
		// element.
		return next == nil || isOneOf(next, atom.Rt, atom.Rp)

	case atom.Optgroup:
		// An optgroup element's end tag may be omitted if the optgroup element is
		// immediately followed by another optgroup element, or if there is no more
		// content in the parent element.
		return next == nil || isOneOf(next, atom.Optgroup)

	case atom.Option:
		// An option element's end tag may be omitted if the option element is
		// immediately followed by another option element, or if it is immediately
		// followed by an optgroup element, or if there is no more content in the parent
		// element.
		return next == nil || isOneOf(next, atom.Option, atom.Optgroup)

	case atom.Colgroup:
		// A colgroup element's end tag may be omitted if the colgroup element is not
		// immediately followed by a space character or a comment.
		return !firstCharIsSpace(next)

	case atom.Thead:
		// A thead element's end tag may be omitted if the thead element is immediately
		// followed by a tbody or tfoot element.
		return isOneOf(next, atom.Tbody, atom.Tfoot)

	case atom.Tbody:
		// A tbody element's end tag may be omitted if the tbody element is immediately
		// followed by a tbody or tfoot element, or if there is no more content in the
		// parent element.
		return next == nil || isOneOf(next, atom.Tbody, atom.Tfoot)

	case atom.Tfoot:
		// A tfoot element's end tag may be omitted if the tfoot element is immediately
		// followed by a tbody element, or if there is no more content in the parent
		// element.
		return next == nil || isOneOf(next, atom.Tbody)

	case atom.Tr:
		// A tr element's end tag may be omitted if the tr element is immediately
		// followed by another tr element, or if there is no more content in the parent
		// element.
		return next == nil || isOneOf(next, atom.Tr)

	case atom.Td:
		// A td element's end tag may be omitted if the td element is immediately
		// followed by a td or th element, or if there is no more content in the parent
		// element.
		return next == nil || isOneOf(next, atom.Td, atom.Th)

	case atom.Th:
		// A th element's end tag may be omitted if the th element is immediately
		// followed by a td or th element, or if there is no more content in the parent
		// element.
		return next == nil || isOneOf(next, atom.Td, atom.Th)

	default:
		return false
	}
}

func render(root *html.Node, buf *strings.Builder, flags HTMLCompressFlag) {
	for currNode := root; currNode != nil; currNode = currNode.NextSibling {
		switch currNode.Type {
		default:
			html.Render(buf, currNode)

		case html.ErrorNode:
			panic(fmt.Sprintf("%s", currNode.Data))

		case html.DoctypeNode:
			html.Render(buf, currNode)

		case html.DocumentNode:
			if !canElideOpener(currNode, flags) {
				html.Render(buf, currNode)
			}

		case html.CommentNode:
			if flags&HTMLComments == 0 {
				html.Render(buf, currNode)
			}

		case html.TextNode:
			if flags&HTMLWhitespace == 0 {
				html.Render(buf, currNode)

			} else if len(strings.TrimSpace(currNode.Data)) != 0 {
				buf.WriteString(reSpaces.ReplaceAllString(currNode.Data, " "))
			}

		case html.ElementNode:
			if flags&HTMLStartTags == 0 || !canElideOpener(currNode, flags) {
				buf.WriteByte('<')
				buf.WriteString(currNode.Data)

				for _, attr := range sortAttrs(currNode.Attr) {
					buf.WriteString(attr.Key)
					buf.WriteByte('=')

					// TODO: Need to omit quotes if compress flag is present, and if there's no
					// 			marker within. Should also retain the original quote style used.
					buf.WriteString(strconv.Quote(attr.Val))
				}

				buf.WriteByte('>')
			}

			render(currNode.FirstChild, buf, flags)

			if flags&HTMLEndTags == 0 || !canElideCloser(currNode, flags) {
				buf.WriteString("</")
				buf.WriteString(currNode.Data)
				buf.WriteByte('>')
			}
		}
	}
}

func sortAttrs(attrs []html.Attribute) []html.Attribute {
	sort.Slice(attrs, func(i, j int) bool {
		var attrI = attrs[i].Key
		var attrJ = attrs[j].Key

		var iIsDataAttr = strings.HasPrefix(attrI, "data-")
		var jIsDataAttr = strings.HasPrefix(attrJ, "data-")

		if iIsDataAttr != jIsDataAttr {
			return jIsDataAttr
		}

		return strings.Compare(attrI, attrJ) < 0
	})

	return attrs
}
