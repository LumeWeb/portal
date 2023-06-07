package main

import (
	"embed"
	"git.lumeweb.com/LumeWeb/portal/config"
	"git.lumeweb.com/LumeWeb/portal/controller"
	"git.lumeweb.com/LumeWeb/portal/db"
	_ "git.lumeweb.com/LumeWeb/portal/docs"
	"git.lumeweb.com/LumeWeb/portal/logger"
	"git.lumeweb.com/LumeWeb/portal/service/files"
	"git.lumeweb.com/LumeWeb/portal/tus"
	validation "github.com/go-ozzo/ozzo-validation"
	"github.com/iris-contrib/swagger"
	"github.com/iris-contrib/swagger/swaggerFiles"
	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/mvc"
	"go.uber.org/zap"
	"log"
	"net/http"
)

// Embed a directory of static files for serving from the app's root path
//
//go:embed app/*
var embedFrontend embed.FS

//	@title			Lume Web Portal
//	@version		1.0
//	@description	A decentralized data storage portal for the open web

//	@contact.name	Lume Web Project
//	@contact.url	https://lumeweb.com
//	@contact.email	contact@lumeweb.com

//	@license.name	MIT
//	@license.url	https://opensource.org/license/mit/

// @externalDocs.description	OpenAPI
// @externalDocs.url			https://swagger.io/resources/open-api/
func main() {
	// Initialize the configuration settings
	config.Init()

	// Initialize the database connection
	db.Init()

	logger.Init()
	files.Init()

	// Create a new Iris app instance
	app := iris.New()

	app.Validator = ozzValidator{}
	// Enable Gzip compression for responses
	app.Use(iris.Compression)

	// Serve static files from the embedded directory at the app's root path
	app.HandleDir("/", embedFrontend)

	api := app.Party("/api")
	v1 := api.Party("/v1")

	// Register the AccountController with the MVC framework and attach it to the "/api/account" path
	mvc.Configure(v1.Party("/account"), func(app *mvc.Application) {
		app.Handle(new(controller.AccountController))
	})

	mvc.Configure(v1.Party("/auth"), func(app *mvc.Application) {
		app.Handle(new(controller.AuthController))
	})

	mvc.Configure(v1.Party("/files"), func(app *mvc.Application) {
		app.Handle(new(controller.FilesController))
	})

	tusHandler := tus.Init()

	v1.Any(tus.TUS_API_PATH+"/{fileparam:path}", iris.FromStd(http.StripPrefix(v1.GetRelPath()+tus.TUS_API_PATH+"/", tusHandler)))
	v1.Post(tus.TUS_API_PATH, iris.FromStd(http.StripPrefix(v1.GetRelPath()+tus.TUS_API_PATH, tusHandler)))

	swaggerConfig := swagger.Config{
		// The url pointing to API definition.
		URL:          "http://localhost:8080/swagger/doc.json",
		DeepLinking:  true,
		DocExpansion: "list",
		DomID:        "#swagger-ui",
		// The UI prefix URL (see route).
		Prefix: "/swagger",
	}
	swaggerUI := swagger.Handler(swaggerFiles.Handler, swaggerConfig)

	app.Get("/swagger", swaggerUI)
	// And the wildcard one for index.html, *.js, *.css and e.t.c.
	app.Get("/swagger/{any:path}", swaggerUI)

	// Start the Iris app and listen for incoming requests on port 80
	err := app.Listen(":8080", func(app *iris.Application) {
		routes := app.GetRoutes()
		for _, route := range routes {
			log.Println(route)
		}
	})

	if err != nil {
		logger.Get().Error("Failed starting webserver proof", zap.Error(err))
	}
}

type ozzValidator struct{}

func (o ozzValidator) Struct(d interface{}) error {
	v, ok := d.(validation.Validatable)

	if !ok {
		return nil
	}

	return v.Validate()
}
