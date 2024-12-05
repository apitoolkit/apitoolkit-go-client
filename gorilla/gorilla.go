package apitoolkitgorilla

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"

	apt "github.com/apitoolkit/apitoolkit-go"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"go.opentelemetry.io/otel/trace"
)

type Config struct {
	Debug               bool
	ServiceVersion      string
	ServiceName         string
	RedactHeaders       []string
	RedactRequestBody   []string
	RedactResponseBody  []string
	Tags                []string
	CaptureRequestBody  bool
	CaptureResponseBody bool
	Tracer              trace.Tracer
}

func ReportError(ctx context.Context, err error) {
	apt.ReportError(ctx, err)
}

// GorillaMuxMiddleware is for the gorilla mux routing library and collects request, response parameters and publishes the payload
func Middleware(config Config) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
			msgID := uuid.Must(uuid.NewRandom())
			newCtx := context.WithValue(req.Context(), apt.CurrentRequestMessageID, msgID)

			errorList := []apt.ATError{}
			newCtx = context.WithValue(newCtx, apt.ErrorListCtxKey, &errorList)
			_, span := config.Tracer.Start(newCtx, string(apt.SpanName))
			newCtx = context.WithValue(newCtx, apt.CurrentSpan, span)
			req = req.WithContext(newCtx)

			reqBuf, _ := io.ReadAll(req.Body)
			req.Body.Close()
			req.Body = io.NopCloser(bytes.NewBuffer(reqBuf))

			rec := httptest.NewRecorder()
			next.ServeHTTP(rec, req)

			recRes := rec.Result()
			for k, v := range recRes.Header {
				for _, vv := range v {
					res.Header().Add(k, vv)
				}
			}
			resBody, _ := io.ReadAll(recRes.Body)
			res.WriteHeader(recRes.StatusCode)
			res.Write(resBody)

			route := mux.CurrentRoute(req)
			pathTmpl, _ := route.GetPathTemplate()
			vars := mux.Vars(req)
			aptConfig := apt.Config{
				ServiceName:         config.ServiceName,
				ServiceVersion:      config.ServiceVersion,
				Tags:                config.Tags,
				Debug:               config.Debug,
				CaptureRequestBody:  config.CaptureRequestBody,
				CaptureResponseBody: config.CaptureResponseBody,
				RedactHeaders:       config.RedactHeaders,
				RedactRequestBody:   config.RedactRequestBody,
				RedactResponseBody:  config.RedactResponseBody,
				Tracer:              config.Tracer,
			}

			payload := apt.BuildPayload(apt.GoGorillaMux,
				req, recRes.StatusCode,
				reqBuf, resBody, recRes.Header, vars, pathTmpl,
				config.RedactHeaders, config.RedactRequestBody, config.RedactResponseBody,
				errorList,
				msgID,
				nil,
				aptConfig,
			)
			apt.CreateSpan(payload, aptConfig, span)

		})
	}
}
