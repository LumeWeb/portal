package main

import (
	"embed"
	"git.lumeweb.com/LumeWeb/portal/config"
	"git.lumeweb.com/LumeWeb/portal/db"
	"git.lumeweb.com/LumeWeb/portal/renterd"
	"git.lumeweb.com/LumeWeb/portal/service"
	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/mvc"
	"log"
)

// Embed a directory of static files for serving from the app's root path
//go:embed app/*
var embedFrontend embed.FS

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

	// Start the renterd process in a goroutine
	go renterd.Main()

	// Start the Iris app and listen for incoming requests on port 80
	log.Fatal(app.Listen(":80"))
}
