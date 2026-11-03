package facturacion

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"
)

// Documentos autorizados (GET /api/facturas/{id}/{formato} de Formi) y el campo
// file de factura_cabecera donde se guardan.
var documentos = []struct{ formato, campo string }{
	{"pdf", "pdf_file"},
	{"xml", "xml_file"},
}

// resultadoDocumentos indica, por formato, si se guardó o por qué no.
type resultadoDocumentos struct {
	Guardados []string          `json:"guardados"`
	Omitidos  []string          `json:"omitidos"` // ya existían y no se pidió forzar
	Errores   map[string]string `json:"errores,omitempty"`
}

// guardarDocumentos descarga de Formi el RIDE (PDF) y el XML autorizado y los
// guarda como archivos en pdf_file / xml_file. Con forzar=false solo baja los que
// falten. Un fallo en un formato no impide guardar el otro.
func guardarDocumentos(ctx context.Context, app core.App, cabecera *core.Record, forzar bool) (*resultadoDocumentos, error) {
	res := &resultadoDocumentos{Guardados: []string{}, Omitidos: []string{}, Errores: map[string]string{}}

	formiID := int64(cabecera.GetInt("formi_id"))
	if formiID == 0 {
		return nil, fmt.Errorf("la factura no tiene formi_id")
	}
	nombreBase := cabecera.GetString("clave_acceso")
	if nombreBase == "" {
		nombreBase = fmt.Sprintf("factura_%d", formiID)
	}

	for _, d := range documentos {
		if !forzar && cabecera.GetString(d.campo) != "" {
			res.Omitidos = append(res.Omitidos, d.formato)
			continue
		}
		llamada, err := descargarDocumento(ctx, formiID, d.formato)
		if err != nil {
			res.Errores[d.formato] = err.Error()
			continue
		}
		if llamada.Status != http.StatusOK || len(llamada.Body) == 0 {
			res.Errores[d.formato] = fmt.Sprintf("Formi respondió %d: %s", llamada.Status, recortar(string(llamada.Body), 300))
			continue
		}
		file, err := filesystem.NewFileFromBytes(llamada.Body, nombreArchivo(llamada, nombreBase+"."+d.formato))
		if err != nil {
			res.Errores[d.formato] = err.Error()
			continue
		}
		cabecera.Set(d.campo, file)
		res.Guardados = append(res.Guardados, d.formato)
	}

	if len(res.Guardados) > 0 {
		if err := app.Save(cabecera); err != nil {
			return nil, err
		}
	}
	if len(res.Errores) == 0 {
		res.Errores = nil
	}
	return res, nil
}

// guardarDocumentosEnSegundoPlano se usa desde el webhook: Formi no tiene que
// esperar las descargas. Si falla, queda en el log y se recupera con /documentos.
func guardarDocumentosEnSegundoPlano(app core.App, cabeceraID string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		cabecera, err := app.FindRecordById("factura_cabecera", cabeceraID)
		if err != nil {
			app.Logger().Error("documentos factura: no se encontró la factura", "factura", cabeceraID, "error", err)
			return
		}
		res, err := guardarDocumentos(ctx, app, cabecera, false)
		if err != nil {
			app.Logger().Error("documentos factura: no se pudieron guardar", "factura", cabeceraID, "error", err)
			return
		}
		if res.Errores != nil {
			app.Logger().Warn("documentos factura: descarga incompleta (usar /api/facturacion/documentos)",
				"factura", cabeceraID, "errores", res.Errores)
		}

		// Correo al cliente solo cuando el PDF se acaba de guardar en esta pasada:
		// si el webhook se repite y los archivos ya estaban, no se reenvía.
		if !slices.Contains(res.Guardados, "pdf") {
			return
		}
		destino, err := enviarFacturaPorCorreo(app, cabecera, "")
		if err != nil {
			app.Logger().Warn("correo factura: no se envió (usar /api/facturacion/enviar-correo)",
				"factura", cabeceraID, "error", err)
			return
		}
		app.Logger().Info("correo factura: enviado", "factura", cabeceraID, "a", destino)
	}()
}

// nombreArchivo toma el nombre de Content-Disposition (Formi lo nombra con la
// clave de acceso) o usa el de respaldo.
func nombreArchivo(l *llamadaFormi, respaldo string) string {
	if _, params, err := mime.ParseMediaType(l.Header.Get("Content-Disposition")); err == nil {
		if n := strings.TrimSpace(params["filename"]); n != "" {
			return n
		}
	}
	return respaldo
}

func recortar(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
