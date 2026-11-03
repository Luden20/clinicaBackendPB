package facturacion

import (
	"fmt"
	"strings"

	"pocketbaseCustom/internal/utils/email"

	"github.com/pocketbase/pocketbase/core"
)

// Plantilla (colección "plantillas") del correo con la factura.
// Variables: {{cliente}}, {{titulo}} (001-002-000000015), {{secuencial}},
// {{claveAcceso}}, {{fechaAutorizacion}}.
const slugPlantillaFactura = "factura"

// enviarFacturaPorCorreo manda el RIDE (PDF) y el XML autorizados como adjuntos.
// destino vacío → usa el correo del cliente. Devuelve el correo al que se envió.
// Si la plantilla "factura" tiene enviar=false no se envía nada (y no es error).
func enviarFacturaPorCorreo(app core.App, cabecera *core.Record, destino string) (string, error) {
	if errs := app.ExpandRecord(cabecera, []string{"cliente"}, nil); len(errs) > 0 {
		return "", fmt.Errorf("expandiendo cliente: %v", errs)
	}
	cliente := cabecera.ExpandedOne("cliente")
	if cliente == nil {
		return "", fmt.Errorf("la factura no tiene cliente")
	}

	destino = strings.TrimSpace(destino)
	if destino == "" {
		destino = strings.TrimSpace(cliente.GetString("correo"))
	}
	if destino == "" {
		return "", fmt.Errorf("el cliente no tiene correo")
	}

	nombreBase := "factura_" + cabecera.GetString("secuencial")
	if cabecera.GetString("secuencial") == "" {
		nombreBase = "factura_" + cabecera.Id
	}

	pdf, err := email.AdjuntoDesdeRecord(app, cabecera, "pdf_file")
	if err != nil {
		return "", err
	}
	pdf.Nombre = nombreBase + ".pdf"
	pdf.ContentType = "application/pdf"

	adjuntos := []email.Adjunto{pdf}
	// El XML es opcional: si no está, se manda solo el PDF.
	if xml, err := email.AdjuntoDesdeRecord(app, cabecera, "xml_file"); err == nil {
		xml.Nombre = nombreBase + ".xml"
		xml.ContentType = "application/xml"
		adjuntos = append(adjuntos, xml)
	}

	nombre := cliente.GetString("nombre")
	if nombre == "" {
		nombre = cliente.GetString("razon_social")
	}
	err = email.SendEmailConAdjuntos(app, []*string{&destino}, slugPlantillaFactura, map[string]string{
		"cliente":           nombre,
		"titulo":            numeroFactura(app, cabecera),
		"nombre":            nombre,
		"secuencial":        cabecera.GetString("secuencial"),
		"claveAcceso":       cabecera.GetString("clave_acceso"),
		"fechaAutorizacion": cabecera.GetDateTime("fecha_autorizacion").Time().In(zonaEcuador).Format("2006-01-02 15:04"),
	}, adjuntos)
	if err != nil {
		return "", err
	}
	return destino, nil
}

// numeroFactura arma el número como lo muestra el SRI: 001-002-000000015.
// Si no hay parámetros de facturación, devuelve solo el secuencial.
func numeroFactura(app core.App, cabecera *core.Record) string {
	secuencial := cabecera.GetString("secuencial")
	parametros, err := app.FindFirstRecordByFilter("parametros_facturacion", "")
	if err != nil {
		return secuencial
	}
	return fmt.Sprintf("%03d-%03d-%s",
		parametros.GetInt("codigo_establecimiento"),
		parametros.GetInt("punto_emision"),
		secuencial)
}
