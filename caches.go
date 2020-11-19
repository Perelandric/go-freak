package freak

import (
	"bytes"
	"sort"
	"sync"
)

/*

TODO: No strings are being cached, so go back and enable this.

It reuses strings references to eliminate redundency in memory.

*/

var stringCache []*[]byte
var stringCacheMux sync.Mutex

// Inserts the string pointer in length-order
func stringCacheInsert(buf *bytes.Buffer) (str []byte) {
	var bufLen = buf.Len()
	if bufLen == 0 {
		return nil
	}

	str = append(make([]byte, 0, bufLen), buf.Bytes()...)

	stringCacheMux.Lock()

	// TODO: Maintain length order (using binary search) and remove the sort below
	stringCache = append(stringCache, &str)

	stringCacheMux.Unlock()

	buf.Reset()

	return str
}

// Is run after all Prototypes have been initialized.
// Perhaps run again as new Prototypes are created if allowed during runtime.
func locateSubstrings() {
	stringCacheMux.Lock() // Locking is probably not necessary, but doesn't hurt.
	defer stringCacheMux.Unlock()

	// TODO: Remove this sort when the above code gets updated.
	sort.Slice(stringCache, func(i, j int) bool {
		return len(*stringCache[i]) < len(*stringCache[j])
	})

	// Staring with the short strings (which are at the start), try to find a matching substring
	// in one of the strings that are equal length or longer (searching longest strings first).
	for i, last := 0, len(stringCache)-1; i < last; i++ {
		for j := last; j > i; j-- {
			var strI, strJ = *stringCache[i], *stringCache[j]
			var idx = bytes.Index(strJ, strI)

			if idx != -1 { // Update the `i` string with the matching substring at `j`
				*stringCache[i] = strJ[idx : idx+len(strI)]
				break
			}
		}
	}
}
