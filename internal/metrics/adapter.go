package metrics

import (
	"bytes"
	"io"
	"net/http"

	"github.com/gofiber/fiber/v3"
	"github.com/valyala/fasthttp"
)

// Bridging between fasthttp (Fiber) and net/http (promhttp).
// promhttp.Handler returns a stdlib http.Handler, so we shim a minimal
// ResponseWriter and *http.Request around the fasthttp context.

// reqWriter buffers headers set via Header() and flushes them into the
// real fasthttp response on the first Write/WriteHeader call — matching
// how net/http.ResponseWriter is actually meant to behave. The earlier
// version returned a fresh copy of the response headers from every
// Header() call, so anything promhttp wrote into it (notably
// Content-Encoding: gzip, which it sets whenever the scraper sends
// Accept-Encoding: gzip — which every real Prometheus does) was silently
// discarded: the body still came out gzip-compressed, but with no
// Content-Encoding header telling the client to decompress it, so
// Prometheus failed to parse every scrape with "expected a valid start
// token, got \x1f" (the gzip magic byte). Confirmed via
// `curl -H "Accept-Encoding: gzip" /metrics` returning gzip bytes with no
// Content-Encoding header, and a real Prometheus instance failing to
// scrape this app at all until this fix.
type reqWriter struct {
	c         fiber.Ctx
	header    http.Header
	wroteHead bool
}

func newReqWriter(c fiber.Ctx) *reqWriter {
	return &reqWriter{c: c, header: http.Header{}}
}

func (rw *reqWriter) Header() http.Header {
	return rw.header
}

func (rw *reqWriter) flushHeader() {
	if rw.wroteHead {
		return
	}
	rw.wroteHead = true
	for k, values := range rw.header {
		for _, v := range values {
			rw.c.Response().Header.Add(k, v)
		}
	}
}

func (rw *reqWriter) Write(b []byte) (int, error) {
	rw.flushHeader()
	return rw.c.Response().BodyWriter().Write(b)
}

func (rw *reqWriter) WriteHeader(statusCode int) {
	rw.flushHeader()
	rw.c.Status(statusCode)
}

func reqFromFasthttp(ctx *fasthttp.RequestCtx) *http.Request {
	req, _ := http.NewRequest(string(ctx.Method()), string(ctx.RequestURI()), io.NopCloser(bytes.NewReader(ctx.Request.Body())))
	ctx.Request.Header.VisitAll(func(k, v []byte) {
		req.Header.Add(string(k), string(v))
	})
	return req
}
