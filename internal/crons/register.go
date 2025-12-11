package crons

import (
	"pocketbaseCustom/internal/crons/recordatorios"

	"github.com/pocketbase/pocketbase/core"
)

func Register(app core.App) {
	app.Cron().MustAdd("recordatorios_vacunas", "0 10 * * *", func() {
		app.Logger().Info("Enviando recordatorios de vacunas diarios")
		recordatorios.Recordatorios(app, 7)

	})
}
