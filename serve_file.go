package freak

import (
	"compress/gzip"
	"crypto/sha1"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

//const long_cache_control = "max-age=31536000, s-maxage=31536000"

const (
	_acceptEncoding  = "Accept-Encoding"
	_contentEncoding = "Content-Encoding"
	_contentType     = "Content-Type"
	_gzip            = "gzip"
	_eTag            = "Etag"
	_ifNoneMatch     = "If-None-Match"
	_msie6           = "MSIE 6"
	_userAgent       = "User-Agent"

	_htmlContent = "text/html; charset=utf-8"

	deadline    = 10 * time.Second
	respBufSize = 2048

	bytesSizeInPool   = 512
	eTagClearSchedule = 5 * time.Minute

	pathSeparatorNeedsConversion = os.PathSeparator != '/'
)

func logErr(err error) bool {
	if err != nil && err != io.EOF {
		log.Println(err.Error())
		return true
	}
	return false
}

func loggingFileRemover(pth string) {
	if err := os.Remove(pth); err != nil {
		const msg = "Error removing file at: %q\n    ERROR: %s"
		log.Printf(msg, pth, err.Error())
	}
}
func loggingCloser(c io.Closer, pth string) {
	if err := c.Close(); err != nil {
		if pth == "" {
			const msg = "Error closing item\n    ERROR: %s"
			log.Printf(msg, err.Error())
		} else {
			const msg = "Error closing file at: %q\n    ERROR: %s"
			log.Printf(msg, pth, err.Error())
		}
	}
}

var fileExt = map[string]struct {
	canGzip bool
	mime    []string
}{
	".css":  {true, []string{"text/css; charset=utf-8"}},
	".htm":  {true, []string{_htmlContent}},
	".html": {true, []string{_htmlContent}},
	".js":   {true, []string{"application/x-javascript"}},
	".json": {true, []string{"application/json"}},
	".xml":  {true, []string{"text/xml; charset=utf-8"}},
	".gif":  {false, []string{"image/gif"}},
	".jpg":  {false, []string{"image/jpeg"}},
	".pdf":  {false, []string{"application/pdf"}},
	".png":  {false, []string{"image/png"}},
}

var (
	htmlContentHeader = fileExt[".html"].mime
	gzipHeader        = []string{_gzip}
)

// Serves a static file from the filesystem for the given path.
// If the content type can be zipped, it first looks for a pre-zipped version
// of the file. If none is found, it attempts to zip and save the file.
func (s *server) serveFile(resp http.ResponseWriter, req *http.Request, urlpath string) {
	switch req.Method {
	case "GET", "POST": // Do nothing
	default:
		http.NotFound(resp, req)
		return
	}

	etg := req.Header.Get(_ifNoneMatch)

	if etg != "" {
		if _, ok := eTagMap.Load(etg); ok {
			resp.WriteHeader(http.StatusNotModified)
			return
		}
	}

	if pathSeparatorNeedsConversion {
		urlpath = filepath.FromSlash(urlpath)
	}

	urlpath = filepath.Join(s.binaryPath, urlpath)

	var urlpathgz string
	var err error

	var doGzip = false
	var respHdrs = resp.Header()
	var file = (*os.File)(nil)
	var buf = []byte(nil)

	if info, ok := fileExt[filepath.Ext(urlpath)]; ok {
		respHdrs[_contentType] = info.mime

		doGzip = info.canGzip &&
			strings.Contains(req.Header.Get(_acceptEncoding), _gzip)

		// First look for a gzip version of the file, if possible
		if doGzip {
			urlpathgz = urlpath + ".gz"

			if file, err = os.Open(urlpathgz); err == nil {
				respHdrs[_contentEncoding] = gzipHeader

				goto SEND_FILE
			}
		}
	}

	// No file yet, so try with the original path
	if file, err = os.Open(urlpath); err != nil {
		http.NotFound(resp, req) // No original, so 404.
		return
	}

	buf = bytesPool.Get().([]byte)
	defer bytesPool.Put(buf[0:0])

	if doGzip { // Gzip the file to a new file and close, reopen, and send it.
		var gzFile *os.File

		if gzFile, err = os.Create(urlpathgz); err == nil {
			var z *gzip.Writer

			z, err = gzip.NewWriterLevel(gzFile, gzip.BestCompression)
			if logErr(err) {
				doGzip = false
				loggingCloser(gzFile, urlpathgz)
				loggingFileRemover(urlpathgz)
				goto SEND_FILE // Original file is still open, so send that one
			}

			for tw := io.TeeReader(file, z); err == nil; {
				_, err = tw.Read(buf[0:bytesSizeInPool])
			}

			loggingCloser(z, "")             // Flush the gzipper...
			loggingCloser(gzFile, urlpathgz) // ...then close the new file...
			loggingCloser(file, urlpath)     // ...and close the original

			if !logErr(err) {
				doGzip = false
				if file, err = os.Open(urlpathgz); !logErr(err) { // Open gzip file
					respHdrs[_contentEncoding] = gzipHeader
					goto SEND_FILE
				}
			}

			// Zipping failed, so grab the original again
			if file, err = os.Open(urlpath); logErr(err) { // Unable to reopen file
				http.Error(
					resp, "Internal server error", http.StatusInternalServerError,
				)
				return
			}
		}
	}

SEND_FILE:
	if doGzip {
		defer loggingCloser(file, urlpathgz)
	} else {
		defer loggingCloser(file, urlpath)
	}

	stat, err := file.Stat()
	if logErr(err) {
		http.Error(resp, "Internal server error", http.StatusInternalServerError)
		return
	}

	if stat.IsDir() { // Make sure the file isn't a directory
		http.Error(resp, "Not authorized", http.StatusUnauthorized)
		return
	}

	sum := sha1.Sum(append(buf[0:0], urlpath...))

	// Format of etag relates to the following indexes:
	//   "               0
	//   shasum          1-40
	//   .				 41
	//   64-bit mod time 42-?
	//   64-bit size	 ?
	//   "				 ?

	mod := stat.ModTime()

	buf = appendHexPair(appendHexPair(appendHexPair(appendHexPair(appendHexPair(
		appendHexPair(appendHexPair(appendHexPair(appendHexPair(appendHexPair(
			appendHexPair(appendHexPair(appendHexPair(appendHexPair(appendHexPair(
				appendHexPair(appendHexPair(appendHexPair(appendHexPair(appendHexPair(
					append(buf, '"'),
					sum[0]), sum[1]), sum[2]), sum[3]), sum[4]),
				sum[5]), sum[6]), sum[7]), sum[8]), sum[9]),
			sum[10]), sum[11]), sum[12]), sum[13]), sum[14]),
		sum[15]), sum[16]), sum[17]), sum[18]), sum[19],
	)

	//buf = append(append(append(append(append(append(append(append(
	//	append(append(append(append(append(append(append(append(append(
	//	append(append(append(append(
	//	buf[0:0], '"'),
	//	toHex(sum[0]>>4), toHex(sum[0]&0xF)),
	//	toHex(sum[1]>>4), toHex(sum[1]&0xF)),
	//	toHex(sum[2]>>4), toHex(sum[2]&0xF)),
	//	toHex(sum[3]>>4), toHex(sum[3]&0xF)),
	//	toHex(sum[4]>>4), toHex(sum[4]&0xF)),
	//	toHex(sum[5]>>4), toHex(sum[5]&0xF)),
	//	toHex(sum[6]>>4), toHex(sum[6]&0xF)),
	//	toHex(sum[7]>>4), toHex(sum[7]&0xF)),
	//	toHex(sum[8]>>4), toHex(sum[8]&0xF)),
	//	toHex(sum[9]>>4), toHex(sum[9]&0xF)),
	//	toHex(sum[10]>>4), toHex(sum[10]&0xF)),
	//	toHex(sum[11]>>4), toHex(sum[11]&0xF)),
	//	toHex(sum[12]>>4), toHex(sum[12]&0xF)),
	//	toHex(sum[13]>>4), toHex(sum[13]&0xF)),
	//	toHex(sum[14]>>4), toHex(sum[14]&0xF)),
	//	toHex(sum[15]>>4), toHex(sum[15]&0xF)),
	//	toHex(sum[16]>>4), toHex(sum[16]&0xF)),
	//	toHex(sum[17]>>4), toHex(sum[17]&0xF)),
	//	toHex(sum[18]>>4), toHex(sum[18]&0xF)),
	//	toHex(sum[19]>>4), toHex(sum[19]&0xF))

	buf = append(int64ToHex(int64ToHex(append(buf,
		'.'), mod.Unix()), stat.Size()), '"',
	)

	var dataStr = string(buf) // need a copy of the reusable buffer

	eTagMap.Store(dataStr, struct{}{})

	if etg == dataStr[1:len(dataStr)-1] {
		resp.WriteHeader(http.StatusNotModified)
		return
	}

	resp.Header().Set(_eTag, dataStr)
	http.ServeContent(resp, req, urlpath, mod, file)
}

var eTagMap sync.Map

// The eTagMap gets cleared on an interval just in case some resource was
// updated. This just means that the tag will be regenerated in the map at
// its next request. If nothing changed, the tag will be the same.
func init() {
	var firstInterval = int64(eTagClearSchedule) - (time.Now().UnixNano() % int64(eTagClearSchedule))

	_ = time.AfterFunc(time.Duration(firstInterval), clearETagMap)
}

func clearETagMap() {
	eTagMap.Range(func(k, _ interface{}) bool {
		eTagMap.Delete(k)
		return true
	})

	_ = time.AfterFunc(eTagClearSchedule, clearETagMap)
}

var bytesPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, 0, bytesSizeInPool)
	},
}

func int64ToHex(buf []byte, n int64) []byte {
	var b = make([]byte, 0, 8)
	for ; n != 0; n >>= 8 {
		b = append([]byte{toHex(byte(n) >> 4), toHex(byte(n) & 0xF)}, b...)
	}
	return append(buf, b...)
}

const hex = "0123456789ABCDEF"

func toHex(b byte) byte {
	return hex[b]
}

func appendHexPair(buf []byte, b byte) []byte {
	return append(buf, hex[b>>4], hex[b&0xF])
}
