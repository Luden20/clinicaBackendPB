package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"pocketbaseCustom/internal/api"
	"pocketbaseCustom/internal/crons"
	"pocketbaseCustom/internal/hooks"
	_ "pocketbaseCustom/migrations"
	"strings"

	"github.com/joho/godotenv"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"
	"github.com/resend/resend-go/v3"

	"pocketbaseCustom/internal/utils"
)

func main() {
	app := pocketbase.New()
	isGoRun := strings.HasPrefix(os.Args[0], os.TempDir())
	ctx := context.TODO()
	err := godotenv.Load()
	if err == nil {
		singleton := utils.GetInstance(os.Getenv("EMAIL_KEY"))
		params := &resend.SendEmailRequest{
			From:    "Clinica Veterinaria Los Chillos <info@clinicaveterinarialoschillos.com\n\n>",
			To:      []string{os.Getenv("EMAIL_INIT_DIR")},
			Subject: "Despliegue exitoso",
			Html:    "<p>Funciona!</p>",
		}

		sent, err := singleton.Client.Emails.SendWithContext(ctx, params)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(sent.Id)
	} else {
		fmt.Println(err)
	}
	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{
		// enable auto creation of migration files when making collection changes in the Dashboard
		// (the isGoRun check is to enable it only during development)
		Automigrate: isGoRun,
	})

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		// serves static files from the provided public dir (if exists)
		se.Router.GET("/{path...}", apis.Static(os.DirFS("./pb_public"), false))

		return se.Next()
	})
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		// register "GET /hello/{name}" route (allowed for everyone)
		se.Router.GET("/hello/{name}", func(e *core.RequestEvent) error {
			name := e.Request.PathValue("name")

			return e.String(http.StatusOK, "Hello "+name)
		})

		// register "POST /api/myapp/settings" route (allowed only for authenticated users)
		se.Router.POST("/api/myapp/settings", func(e *core.RequestEvent) error {
			// do something ...
			return e.JSON(http.StatusOK, map[string]bool{"success": true})
		}).Bind(apis.RequireAuth())

		return se.Next()
	})
	print("iniciando backend clinicaBackendPB")
	api.Register(app)
	hooks.Register(app)
	crons.Register(app)
	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
