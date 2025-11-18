package hooks

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

func Register(app core.App) {
	app.OnBootstrap().BindFunc(func(e *core.BootstrapEvent) error {
		fmt.Println("iniciando")
		if err := e.Next(); err != nil {
			return err
		}
		return nil
	})
}
