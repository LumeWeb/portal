package swagger

import (
	"embed"
	"github.com/gorilla/mux"
	"go.sia.tech/jape"
	"io/fs"
	"log"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
)

//go:generate go run generate.go

//go:embed embed
var swagfs embed.FS

func byteHandler(b []byte) jape.Handler {
	return func(c jape.Context) {
		c.ResponseWriter.Header().Set("Content-Type", "application/json")
		_, err := c.ResponseWriter.Write(b)
		if err != nil {
			log.Println(err)
		}
	}
}

func Swagger(spec []byte, router *mux.Router) error {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(spec)
	if err != nil {
		return err
	}

	if err = doc.Validate(loader.Context); err != nil {
		return err
	}

	jsonDoc, err := doc.MarshalJSON()
	if err != nil {
		return err
	}

	swaggerFiles, _ := fs.Sub(swagfs, "embed")
	swaggerHandler := http.StripPrefix("/swagger", http.FileServer(http.FS(swaggerFiles)))

	router.HandleFunc("/swagger.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jsonDoc)
	}).Methods("GET")

	router.HandleFunc("/swagger", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/swagger/", http.StatusMovedPermanently)
	}).Methods("GET")

	router.PathPrefix("/swagger").Handler(swaggerHandler)

	return nil
}
