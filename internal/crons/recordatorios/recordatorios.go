package recordatorios

import (
	"fmt"
	"pocketbaseCustom/internal/utils/email"
	"strconv"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

func Recordatorios(app core.App, diasRestantes int) {
	objetivo := time.Now().AddDate(0, 0, diasRestantes).Format("2006-01-02")
	pendientes, err := app.FindAllRecords("carnets",
		dbx.NewExp("DATE(reaplicacion) = {:f}", dbx.Params{"f": objetivo}),
	)
	if err != nil {
		fmt.Println(err)
		app.Logger().Error("Error obteniendo datos para recordatorios")
		return
	}
	fmt.Println("Enviando " + strconv.Itoa(len(pendientes)))
	app.Logger().Info("Enviando " + strconv.Itoa(len(pendientes)))
	for _, item := range pendientes {

		mascota, err := app.FindRecordById("mascotas", item.GetString("mascota"))
		if err != nil {
			continue
		}
		cliente, err := app.FindRecordById("clientes", mascota.GetString("cliente"))
		if err != nil {
			continue
		}
		correo := cliente.GetString("correo")
		email.SendEmail(app, []*string{&correo}, "carnet_recordotario", map[string]string{
			"cliente":      cliente.GetString("nombre"),
			"mascota":      mascota.GetString("nombre"),
			"tipo":         item.GetString("tipo"),
			"marca":        item.GetString("marca"),
			"aplicacion":   item.GetDateTime("aplicacion").Time().Format("2006-01-02"),
			"reaplicacion": item.GetDateTime("reaplicacion").Time().Format("2006-01-02"),
			"dias":         strconv.Itoa(diasRestantes),
		})
	}

}
