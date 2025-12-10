package utils

import (
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ses"
)

func InitEmailService() {
	singleton := GetEmailClientInstance()
	fmt.Println("SES CLIENT:", singleton.Client)

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
					Data:    aws.String("Clinica Backend Desplegado en " + os.Getenv("APP_DEV") + " " + time.Now().GoString()),
				},
			},
			Subject: &ses.Content{
				Charset: aws.String("UTF-8"),
				Data:    aws.String(os.Getenv("APP_DEV") + " deploy"),
			},
		},
		Source: aws.String(os.Getenv("EMAIL_ROL") + "@clinicaveterinarialoschillos.com"),
	}

	// Attempt to send the email.
	result, err := singleton.Client.SendEmail(input)
	if err != nil {
		panic(err)
	}
	fmt.Println(result)
}
