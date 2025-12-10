package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"pocketbaseCustom/internal/api"
	"pocketbaseCustom/internal/crons"
	"pocketbaseCustom/internal/hooks"
	_ "pocketbaseCustom/migrations"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ses"
	"github.com/joho/godotenv"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"

	"pocketbaseCustom/internal/utils"
)

func main() {
	app := pocketbase.New()
	isGoRun := strings.HasPrefix(os.Args[0], os.TempDir())
	_ = godotenv.Load()
	singleton := utils.GetInstance()
	rol := os.Getenv("EMAIL_ROL")
	if rol == "" {
		rol = "info"
	}
	input := &ses.SendEmailInput{
		Destination: &ses.Destination{
			CcAddresses: []*string{},
			ToAddresses: []*string{
				aws.String(os.Getenv("EMAIL_INIT_DIR")),
			},
		},
		Message: &ses.Message{
			Body: &ses.Body{
				Html: &ses.Content{
					Charset: aws.String("UTF-8"),
					Data:    aws.String("<h1> Funciona xd </h1>"),
				},
				Text: &ses.Content{
					Charset: aws.String("UTF-8"),
					Data:    aws.String("Funciona awebo"),
				},
			},
			Subject: &ses.Content{
				Charset: aws.String("UTF-8"),
				Data:    aws.String("Prueba local"),
			},
		},
		Source: aws.String("info@clinicaveterinarialoschillos.com"),
	}

	// Attempt to send the email.
	result, err := singleton.Client.SendEmail(input)
	if err != nil {
		panic(err)
	}
	fmt.Println(result)
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
