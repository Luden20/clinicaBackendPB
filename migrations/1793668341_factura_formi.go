package migrations

import (
	"slices"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Campos de factura_cabecera para la integración con Formi (webhook / revisión).
// Idempotente: solo agrega lo que falte, así funciona tanto en la base local
// (donde estado/errores ya se crearon desde el dashboard) como en producción.
func init() {
	m.Register(func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("factura_cabecera")
		if err != nil {
			return err
		}

		if f, ok := col.Fields.GetByName("estado").(*core.SelectField); ok {
			for _, v := range []string{"sin_procesar", "procesada", "enviada_sri", "error_sri", "aprobada_sri"} {
				if !slices.Contains(f.Values, v) {
					f.Values = append(f.Values, v)
				}
			}
		}

		addIfMissing := func(f core.Field) {
			if col.Fields.GetByName(f.GetName()) == nil {
				col.Fields.Add(f)
			}
		}
		addIfMissing(&core.JSONField{Id: "json1689012054", Name: "errores"})
		addIfMissing(&core.NumberField{Id: "number_formi_id", Name: "formi_id", OnlyInt: true})
		addIfMissing(&core.TextField{Id: "text_secuencial", Name: "secuencial"})
		addIfMissing(&core.TextField{Id: "text_clave_acceso", Name: "clave_acceso"})
		addIfMissing(&core.TextField{Id: "text_pdf_url", Name: "pdf_url"})
		addIfMissing(&core.TextField{Id: "text_xml_url", Name: "xml_url"})
		addIfMissing(&core.DateField{Id: "date_fecha_autorizacion", Name: "fecha_autorizacion"})

		return app.Save(col)
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("factura_cabecera")
		if err != nil {
			return err
		}
		for _, name := range []string{"formi_id", "secuencial", "clave_acceso", "pdf_url", "xml_url", "fecha_autorizacion"} {
			col.Fields.RemoveByName(name)
		}
		return app.Save(col)
	})
}
