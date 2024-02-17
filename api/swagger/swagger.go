package swagger

import (
	"embed"
	"io/fs"
	"net/http"

	"git.lumeweb.com/LumeWeb/portal/api/middleware"

	"go.sia.tech/jape"

	"github.com/getkin/kin-openapi/openapi3"
)

//go:generate go run generate.go

//go:embed embed
var swagfs embed.FS

func byteHandler(b []byte) jape.Handler {
	return func(c jape.Context) {
		c.ResponseWriter.Header().Set("Content-Type", "application/json")
		c.ResponseWriter.Write(b)
	}
}

func Swagger(spec []byte, routes map[string]jape.Handler) (map[string]jape.Handler, error) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(spec)
	if err != nil {
		return nil, err
	}

	if err = doc.Validate(loader.Context); err != nil {
		return nil, err
	}

	jsonDoc, err := doc.MarshalJSON()

	if err != nil {
		return nil, err
	}

	swaggerFiles, _ := fs.Sub(swagfs, "embed")
	swaggerServ := http.FileServer(http.FS(swaggerFiles))
	handler := func(c jape.Context) {
		swaggerServ.ServeHTTP(c.ResponseWriter, c.Request)
	}

	strip := func(next http.Handler) http.Handler {
		return http.StripPrefix("/swagger", next)
	}

	redirect := func(jc jape.Context) {
		http.Redirect(jc.ResponseWriter, jc.Request, "/swagger/", http.StatusMovedPermanently)
	}

	swagRoutes := map[string]jape.Handler{
		"GET /swagger.json":  byteHandler(jsonDoc),
		"GET /swagger":       redirect,
		"GET /swagger/*path": middleware.ApplyMiddlewares(handler, strip),
	}

	return middleware.MergeRoutes(routes, swagRoutes), nil
}
