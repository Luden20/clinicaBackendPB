package utils

import (
	"fmt"
	"sync"

	"github.com/resend/resend-go/v3"
)

var lock = &sync.Mutex{}

type email_client struct {
	Client *resend.Client
}

var singleInstance *email_client

func GetInstance(key ...string) *email_client {
	if singleInstance == nil {
		lock.Lock()
		defer lock.Unlock()
		if singleInstance == nil {
			fmt.Println("Creating email_client instance now.")
			singleInstance = &email_client{
				Client: resend.NewClient(key[0]),
			}
		} else {
			fmt.Println("Single instance already created.")
		}
	} else {
		fmt.Println("Single instance already created.")
	}
	return singleInstance
}
