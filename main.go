package main

import (
	"embed"
	"git.lumeweb.com/LumeWeb/portal/config"
	"git.lumeweb.com/LumeWeb/portal/db"
	_ "git.lumeweb.com/LumeWeb/portal/docs"
	"git.lumeweb.com/LumeWeb/portal/service"
	"github.com/iris-contrib/swagger"
	"github.com/iris-contrib/swagger/swaggerFiles"
	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/mvc"
	"log"
)

// Embed a directory of static files for serving from the app's root path
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

//	@externalDocs.description	OpenAPI
//	@externalDocs.url			https://swagger.io/resources/open-api/
func main() {
	// Initialize the configuration settings
	config.Init()

	// Initialize the database connection
	db.Init()

	// Create a new Iris app instance
	app := iris.New()

	// Enable Gzip compression for responses
	app.Use(iris.Compression)

	// Serve static files from the embedded directory at the app's root path
	app.HandleDir("/", embedFrontend)

	// Register the AccountService with the MVC framework and attach it to the "/api/account" path
	mvc.Configure(app.Party("/api/account"), func(app *mvc.Application) {
		app.Handle(new(service.AccountService))
	})

	mvc.Configure(app.Party("/api/auth"), func(app *mvc.Application) {
		app.Handle(new(service.AuthService))
	})

	config := swagger.Config{
		// The url pointing to API definition.
		URL:          "http://localhost:8080/swagger/doc.json",
		DeepLinking:  true,
		DocExpansion: "list",
		DomID:        "#swagger-ui",
		// The UI prefix URL (see route).
		Prefix: "/swagger",
	}
	swaggerUI := swagger.Handler(swaggerFiles.Handler, config)

	app.Get("/swagger", swaggerUI)
	// And the wildcard one for index.html, *.js, *.css and e.t.c.
	app.Get("/swagger/{any:path}", swaggerUI)

	// Start the renterd process in a goroutine
	//go renterd.Main()

	// Start the Iris app and listen for incoming requests on port 80
	log.Fatal(app.Listen(":80"))
}
