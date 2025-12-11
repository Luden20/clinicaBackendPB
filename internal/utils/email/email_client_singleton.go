package email

import (
	"fmt"
	"os"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ses"
)

var lock = &sync.Mutex{}

type Email_client struct {
	Client *ses.SES
}

var singleInstance *Email_client

func GetEmailClientInstance() *Email_client {
	if singleInstance == nil {
		lock.Lock()
		defer lock.Unlock()
		if singleInstance == nil {
			fmt.Println("Creating email_client instance now.")
			sess, err := session.NewSession(&aws.Config{
				Region: aws.String(os.Getenv("AWS_REGION")),
			})
			if err != nil {
				panic(err)
			}
			singleInstance = &Email_client{
				Client: ses.New(sess),
			}
		} else {
			fmt.Println("Single instance already created.")
		}
	} else {
		fmt.Println("Single instance already created.")
	}
	return singleInstance
}
