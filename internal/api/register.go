package api

import (
	"net/http"

	"github.com/pocketbase/pocketbase/core"
)

func Register(app core.App) {
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		grp := se.Router.Group("/api/hello")
		grp.GET("/", func(e *core.RequestEvent) error {
			return e.JSON(http.StatusOK, map[string]string{"msg": "hola"})
		})
		return se.Next()
	})
}
