package freak

import (
	"bytes"
	"io"
	"reflect"
	"sync"
	"unsafe"
)

// TODO: Eventually have different pools for different sizes (to some limit)
var wrapEndingSliceStackPool sync.Pool

func getWrapEndingSliceStack[T any](expectedSize int) [][]*component[T] {
	var v = wrapEndingSliceStackPool.Get()
	var s [][]*component[T]

	if v == nil {
		s = make([][]*component[T], expectedSize, expectedSize)

	} else {
		s = v.([][]*component[T])

		for cap(s) < expectedSize {
			s = append(s, make([]*component[T], 0, 1))
		}

		if len(s) < expectedSize {
			s = s[0:cap(s):cap(s)]
		}
	}

	return s
}

const endingFuncMaxCap = 2
const endingSliceStackMaxCap = 2

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func returnWrapEndingSliceStack[T any](s [][]*component[T]) {
	var maxCap = min(endingSliceStackMaxCap, cap(s))

	s = s[0:maxCap:maxCap]

	for i := range s {
		var maxFuncCap = min(endingFuncMaxCap, cap(s[i]))

		s[i] = s[i][0:maxFuncCap:maxFuncCap]

		for j := range s[i] {
			s[i][j] = nil
		}

		s[i] = s[i][0:0:maxFuncCap]
	}

	wrapEndingSliceStackPool.Put(s)
}

func strToBytes(s string) []byte {
	return *(*[]byte)(unsafe.Pointer(
		&reflect.SliceHeader{
			Data: (*reflect.StringHeader)(unsafe.Pointer(&s)).Data,
			Len:  len(s),
			Cap:  len(s),
		}),
	)
}

func bytesToStr(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

const (
	strNull    = "\uFFFD"
	strDblQuot = "&#34;"
	strSglQuot = "&#39;"
	strAmp     = "&amp;"
	strLssThan = "&lt;"
	strGtrThan = "&gt;"
)

var (
	bytNull    = []byte(strNull)
	bytDblQuot = []byte(strDblQuot)
	bytSglQuot = []byte(strSglQuot)
	bytAmp     = []byte(strAmp)
	bytLssThan = []byte(strLssThan)
	bytGtrThan = []byte(strGtrThan)
)

// writeUnDoubleQuote writes `s` to the Writer but with double quotes escaped
func writeUnDoubleQuote(w io.Writer, s string) {
	var last = 0

	for i := range s {
		if s[i] == '"' {
			w.Write(strToBytes(s[last:i]))
			w.Write(bytDblQuot)

			last = i + 1
		}
	}

	if last != len(s) {
		w.Write(strToBytes(s[last:]))
	}
}

// writeEscapeHTMLString writes `s` to the Writer but escaped for security
func writeEscapeHTMLString(w io.Writer, s string) (n int, err error) {
	return writeEscapeHTMLBytes(w, strToBytes(s))
}

// writeEscapeHTMLString writes `b` to the Writer but escaped for security
func writeEscapeHTMLBytes(w io.Writer, b []byte) (n int, err error) {
	var last = 0
	var escaped []byte
	var written = 0

	for i, c := range b {
		switch c {
		case '\x00':
			escaped = bytNull
		case '"':
			escaped = bytDblQuot
		case '\'':
			escaped = bytSglQuot
		case '&':
			escaped = bytAmp
		case '<':
			escaped = bytLssThan
		case '>':
			escaped = bytGtrThan
		default:
			continue
		}

		n, err = w.Write(b[last:i])
		written += n

		n, err = w.Write(escaped)
		written += n

		last = i + 1
	}

	if escaped == nil { // No writes took place
		return w.Write(b)
	}

	if last != len(b) {
		n, err = w.Write(b[last:])
		return written + n, err
	}
	return written, err
}

// escapeHTMLString returns a string escaped for security. If no escaping was
// needed, there's no allocation and the original string is returned.
func escapeHTMLString(s string) string {
	var last = 0
	var escaped string
	var buf bytes.Buffer

	for i, c := range s {
		switch c {
		case '\x00':
			escaped = strNull
		case '"':
			escaped = strDblQuot
		case '\'':
			escaped = strSglQuot
		case '&':
			escaped = strAmp
		case '<':
			escaped = strLssThan
		case '>':
			escaped = strGtrThan
		default:
			continue
		}

		buf.WriteString(s[last:i])

		buf.WriteString(escaped)
		last = i + 1
	}

	if last != 0 {
		buf.WriteString(s[last:])
		return buf.String()
	}

	return s
}

// escapeHTMLBytes returns a string escaped for security. If no escaping was
// needed, there's no allocation and the original string is returned.
func escapeHTMLBytes(b []byte) []byte {
	var last = 0
	var escaped string
	var buf bytes.Buffer

	for i, c := range b {
		switch c {
		case '\x00':
			escaped = strNull
		case '"':
			escaped = strDblQuot
		case '\'':
			escaped = strSglQuot
		case '&':
			escaped = strAmp
		case '<':
			escaped = strLssThan
		case '>':
			escaped = strGtrThan
		default:
			continue
		}

		buf.Write(b[last:i])

		buf.WriteString(escaped)
		last = i + 1
	}

	if last != 0 {
		buf.Write(b[last:])
		return buf.Bytes()
	}

	return b
}
