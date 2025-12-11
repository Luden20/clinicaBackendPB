package email

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ses"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

func RenderTemplate(html string, data map[string]string) string {
	for key, value := range data {
		html = strings.ReplaceAll(html, "{{"+key+"}}", value)
	}
	return html
}
func SendEmail(app core.App, To []string, slugPlantilla string, data map[string]string) {
	singleton := GetEmailClientInstance()
	plantilla, err := app.FindFirstRecordByFilter(
		"plantillas",
		"slug={:slug}",
		dbx.Params{
			"slug": slugPlantilla,
		})
	if err != nil {
		app.Logger().Error("Plantilla no encotrada para correo " + slugPlantilla)
		return
	}
	contenido := plantilla.GetString("contenido")
	contenidoLleno := RenderTemplate(contenido, data)
	asunto := plantilla.GetString("asunto")
	asuntoLleno := RenderTemplate(asunto, data)
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
					Data:    aws.String(contenidoLleno),
				},
			},
			Subject: &ses.Content{
				Charset: aws.String("UTF-8"),
				Data:    aws.String(asuntoLleno),
			},
		},
		Source: aws.String(os.Getenv("EMAIL_ROL") + "@clinicaveterinarialoschillos.com"),
	}

	result, err := singleton.Client.SendEmail(input)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(result)
}
func InitEmailService(app core.App) {
	destinatario := []string{os.Getenv("EMAIL_INIT_DIR")}
	SendEmail(app, destinatario, "init", map[string]string{
		"ambiente": os.Getenv("APP_DEV"),
		"tiempo":   time.Now().String(),
	})
}
