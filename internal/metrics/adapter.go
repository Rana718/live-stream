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

type reqWriter struct{ c fiber.Ctx }

func (rw reqWriter) Header() http.Header {
	h := http.Header{}
	rw.c.Response().Header.VisitAll(func(k, v []byte) {
		h.Add(string(k), string(v))
	})
	return h
}

func (rw reqWriter) Write(b []byte) (int, error) {
	return rw.c.Response().BodyWriter().Write(b)
}

func (rw reqWriter) WriteHeader(statusCode int) {
	rw.c.Status(statusCode)
}

func reqFromFasthttp(ctx *fasthttp.RequestCtx) *http.Request {
	req, _ := http.NewRequest(string(ctx.Method()), string(ctx.RequestURI()), io.NopCloser(bytes.NewReader(ctx.Request.Body())))
	ctx.Request.Header.VisitAll(func(k, v []byte) {
		req.Header.Add(string(k), string(v))
	})
	return req
}
