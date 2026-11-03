package facturacion

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

func Register(app core.App) {
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		grp := se.Router.Group("/api/facturacion")
		grp.POST("/facturar", facturar).Bind(apis.RequireAuth())
		grp.POST("/revisar", revisar).Bind(apis.RequireAuth())
		grp.POST("/documentos", obtenerDocumentos).Bind(apis.RequireAuth())
		grp.POST("/enviar-correo", enviarCorreo).Bind(apis.RequireAuth())
		grp.POST("/webhook", webhook)
		return se.Next()
	})
}

// enviarCorreo manda (o reenvía) la factura aprobada por correo con el PDF y el
// XML adjuntos, usando la plantilla "factura".
// Body: {"id": "<factura_cabecera.id>", "correo": ""}  (correo vacío → el del cliente)
func enviarCorreo(e *core.RequestEvent) error {
	var body struct {
		ID     string `json:"id"`
		Correo string `json:"correo"`
	}
	if err := e.BindBody(&body); err != nil || strings.TrimSpace(body.ID) == "" {
		return e.BadRequestError("Se espera JSON {\"id\": \"<id de factura_cabecera>\", \"correo\": \"opcional\"}", err)
	}
	cabecera, err := e.App.FindRecordById("factura_cabecera", strings.TrimSpace(body.ID))
	if err != nil {
		return e.NotFoundError("Factura no encontrada", err)
	}
	if cabecera.GetString("estado") != EstadoAprobadaSRI {
		return e.BadRequestError("La factura no está aprobada por el SRI", nil)
	}
	if cabecera.GetString("pdf_file") == "" {
		return e.BadRequestError("La factura no tiene el PDF guardado; usa /documentos primero", nil)
	}

	destino, err := enviarFacturaPorCorreo(e.App, cabecera, body.Correo)
	if err != nil {
		return e.JSON(http.StatusBadGateway, map[string]any{"ok": false, "mensaje": err.Error()})
	}
	return e.JSON(http.StatusOK, map[string]any{"ok": true, "correo": destino})
}

var envioMu sync.Mutex

func leerID(e *core.RequestEvent) (string, error) {
	var body struct {
		ID string `json:"id"`
	}
	if err := e.BindBody(&body); err != nil || strings.TrimSpace(body.ID) == "" {
		return "", e.BadRequestError("Se espera JSON {\"id\": \"<id de factura_cabecera>\"}", err)
	}
	return strings.TrimSpace(body.ID), nil
}

func facturar(e *core.RequestEvent) error {
	id, err := leerID(e)
	if err != nil {
		return err
	}

	envioMu.Lock()
	defer envioMu.Unlock()

	cabecera, err := e.App.FindRecordById("factura_cabecera", id)
	if err != nil {
		return e.NotFoundError("Factura no encontrada", err)
	}
	switch cabecera.GetString("estado") {
	case EstadoEnviadaSRI:
		return e.BadRequestError("La factura ya fue enviada y está en proceso; usa /revisar", nil)
	case EstadoAprobadaSRI:
		return e.BadRequestError("La factura ya está aprobada por el SRI", nil)
	}

	parametros, err := e.App.FindFirstRecordByFilter("parametros_facturacion", "")
	if err != nil {

		return e.BadRequestError("No hay parametros_facturacion configurados", err)
	}

	solicitud, err := construirSolicitud(e.App, cabecera, parametros)
	if err != nil {
		var ev errorValidacion
		if errors.As(err, &ev) {
			return e.BadRequestError(ev.Error(), nil)
		}
		return err
	}

	llamada, err := encolarFactura(e.Request.Context(), solicitud)
	if err != nil {
		cabecera.Set("estado", EstadoErrorSRI)
		cabecera.Set("errores", map[string]any{"origen": "envio", "error": err.Error()})
		if saveErr := e.App.Save(cabecera); saveErr != nil {
			return saveErr
		}
		return e.JSON(http.StatusBadGateway, map[string]any{"ok": false, "estado": EstadoErrorSRI, "mensaje": err.Error()})
	}

	if llamada.Status != http.StatusAccepted {
		cabecera.Set("estado", EstadoErrorSRI)
		cabecera.Set("errores", jsonCrudo(llamada))
		if err := e.App.Save(cabecera); err != nil {
			return err
		}
		return e.JSON(http.StatusBadGateway, map[string]any{
			"ok": false, "estado": EstadoErrorSRI, "formiStatus": llamada.Status, "errores": jsonCrudo(llamada),
		})
	}

	var res respuestaFormi
	if err := json.Unmarshal(llamada.Body, &res); err != nil || res.FacturaID == 0 {
		cabecera.Set("estado", EstadoErrorSRI)
		cabecera.Set("errores", jsonCrudo(llamada))
		if err := e.App.Save(cabecera); err != nil {
			return err
		}
		return e.JSON(http.StatusBadGateway, map[string]any{"ok": false, "estado": EstadoErrorSRI, "mensaje": "respuesta 202 de Formi sin facturaId"})
	}

	// Aceptada: guardar el id de Formi y consumir el secuencial.
	err = e.App.RunInTransaction(func(tx core.App) error {
		cabecera.Set("formi_id", res.FacturaID)
		cabecera.Set("secuencial", solicitud.Factura.Emisor.Secuencial)
		cabecera.Set("estado", EstadoEnviadaSRI)
		cabecera.Set("errores", nil)
		if err := tx.Save(cabecera); err != nil {
			return err
		}
		parametros.Set("secuencial", parametros.GetInt("secuencial")+1)
		return tx.Save(parametros)
	})
	if err != nil {
		return err
	}

	return e.JSON(http.StatusAccepted, map[string]any{
		"ok":         true,
		"estado":     EstadoEnviadaSRI,
		"formiId":    res.FacturaID,
		"secuencial": solicitud.Factura.Emisor.Secuencial,
	})
}

// revisar pregunta a Formi por el estado de la factura y lo aplica.
func revisar(e *core.RequestEvent) error {
	id, err := leerID(e)
	if err != nil {
		return err
	}
	cabecera, err := e.App.FindRecordById("factura_cabecera", id)
	if err != nil {
		return e.NotFoundError("Factura no encontrada", err)
	}
	formiID := int64(cabecera.GetInt("formi_id"))
	if formiID == 0 {
		return e.BadRequestError("La factura nunca fue aceptada por Formi (no tiene formi_id)", nil)
	}

	llamada, err := consultarFactura(e.Request.Context(), formiID)
	if err != nil {
		return e.JSON(http.StatusBadGateway, map[string]any{"ok": false, "mensaje": err.Error()})
	}
	if llamada.Status != http.StatusOK {
		// 401/404/5xx: no cambia el estado, solo se informa.
		return e.JSON(http.StatusBadGateway, map[string]any{
			"ok": false, "formiStatus": llamada.Status, "respuesta": jsonCrudo(llamada),
		})
	}
	var res respuestaFormi
	if err := json.Unmarshal(llamada.Body, &res); err != nil {
		return e.JSON(http.StatusBadGateway, map[string]any{"ok": false, "mensaje": "respuesta de Formi inválida", "respuesta": jsonCrudo(llamada)})
	}
	if err := aplicarResultado(e.App, cabecera, res, llamada.Body); err != nil {
		return err
	}
	respuesta := map[string]any{
		"ok":          cabecera.GetString("estado") == EstadoAprobadaSRI,
		"estado":      cabecera.GetString("estado"),
		"formiEstado": res.Estado,
		"errores":     cabecera.Get("errores"),
	}
	// Aprobada: bajar el PDF y el XML que falten.
	if cabecera.GetString("estado") == EstadoAprobadaSRI {
		docs, err := guardarDocumentos(e.Request.Context(), e.App, cabecera, false)
		if err != nil {
			return err
		}
		respuesta["documentos"] = docs
	}
	return e.JSON(http.StatusOK, respuesta)
}

// obtenerDocumentos vuelve a descargar de Formi el PDF (RIDE) y el XML de una
// factura aprobada y los guarda en pdf_file / xml_file. Sirve para facturas que
// se aprobaron pero cuyos documentos no se llegaron a guardar.
// Body: {"id": "<factura_cabecera.id>", "forzar": false}
// forzar=true reemplaza los archivos aunque ya existan.
func obtenerDocumentos(e *core.RequestEvent) error {
	var body struct {
		ID     string `json:"id"`
		Forzar bool   `json:"forzar"`
	}
	if err := e.BindBody(&body); err != nil || strings.TrimSpace(body.ID) == "" {
		return e.BadRequestError("Se espera JSON {\"id\": \"<id de factura_cabecera>\", \"forzar\": false}", err)
	}
	cabecera, err := e.App.FindRecordById("factura_cabecera", strings.TrimSpace(body.ID))
	if err != nil {
		return e.NotFoundError("Factura no encontrada", err)
	}
	if cabecera.GetString("estado") != EstadoAprobadaSRI {
		return e.BadRequestError("La factura no está aprobada por el SRI; usa /revisar primero", nil)
	}
	if cabecera.GetInt("formi_id") == 0 {
		return e.BadRequestError("La factura no tiene formi_id", nil)
	}

	docs, err := guardarDocumentos(e.Request.Context(), e.App, cabecera, body.Forzar)
	if err != nil {
		return err
	}
	status := http.StatusOK
	if docs.Errores != nil {
		status = http.StatusBadGateway
	}
	return e.JSON(status, map[string]any{
		"ok":         docs.Errores == nil,
		"documentos": docs,
		"pdf_file":   cabecera.GetString("pdf_file"),
		"xml_file":   cabecera.GetString("xml_file"),
	})
}

// webhook recibe facturaFinalizada de Formi. Responde 2XX para que Formi deje
// de reintentar, salvo token inválido o fallo al guardar.
func webhook(e *core.RequestEvent) error {
	token := os.Getenv("FORMI_WEBHOOK_TOKEN")
	if token == "" {
		e.App.Logger().Error("webhook Formi: falta FORMI_WEBHOOK_TOKEN, se rechaza")
		return e.JSON(http.StatusServiceUnavailable, map[string]string{"mensaje": "webhook no configurado"})
	}
	got := e.Request.URL.Query().Get("token")
	if got == "" {
		got = e.Request.Header.Get("X-Webhook-Token")
	}
	if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
		return e.UnauthorizedError("token inválido", nil)
	}

	raw, err := io.ReadAll(io.LimitReader(e.Request.Body, 1<<20))
	if err != nil {
		return e.BadRequestError("No se pudo leer el cuerpo", err)
	}
	var res respuestaFormi
	if err := json.Unmarshal(raw, &res); err != nil || res.FacturaID == 0 {
		e.App.Logger().Warn("webhook Formi: cuerpo inválido", "body", string(raw))
		return e.BadRequestError("JSON inválido o sin facturaId", err)
	}

	cabecera, err := e.App.FindFirstRecordByFilter("factura_cabecera", "formi_id = {:id}", dbx.Params{"id": res.FacturaID})
	if err != nil {
		// Desconocida: se confirma igual para que Formi no reintente para siempre.
		e.App.Logger().Warn("webhook Formi: factura desconocida", "facturaId", res.FacturaID, "body", string(raw))
		return e.JSON(http.StatusOK, map[string]any{"recibido": true, "encontrada": false})
	}
	if err := aplicarResultado(e.App, cabecera, res, raw); err != nil {
		return err // 5xx → Formi reintenta
	}
	if cabecera.GetString("estado") == EstadoAprobadaSRI {
		guardarDocumentosEnSegundoPlano(e.App, cabecera.Id)
	}
	return e.JSON(http.StatusOK, map[string]any{"recibido": true, "estado": cabecera.GetString("estado")})
}

// aplicarResultado traduce el estado de Formi al de factura_cabecera.
//
//	AUTORIZADA           → aprobada_sri (limpia errores)
//	ENCOLADA, PROCESANDO → enviada_sri
//	RECHAZADA, ERROR, ?  → error_sri, con el JSON crudo en "errores"
func aplicarResultado(app core.App, cabecera *core.Record, res respuestaFormi, raw []byte) error {
	actual := cabecera.GetString("estado")
	switch strings.ToUpper(res.Estado) {
	case "AUTORIZADA":
		cabecera.Set("estado", EstadoAprobadaSRI)
		cabecera.Set("errores", nil)
		if res.ClaveAcceso != nil {
			cabecera.Set("clave_acceso", *res.ClaveAcceso)
		}
		if res.FechaAutorizacion != nil {
			cabecera.Set("fecha_autorizacion", *res.FechaAutorizacion)
		}
	case "ENCOLADA", "PROCESANDO":
		if actual == EstadoAprobadaSRI {
			return nil // nunca retroceder una aprobada
		}
		cabecera.Set("estado", EstadoEnviadaSRI)
	default:
		if actual == EstadoAprobadaSRI {
			app.Logger().Warn("Formi informa error sobre una factura ya aprobada; se ignora",
				"factura", cabecera.Id, "body", string(raw))
			return nil
		}
		cabecera.Set("estado", EstadoErrorSRI)
		cabecera.Set("errores", rawJSON(raw))
	}
	return app.Save(cabecera)
}

// jsonCrudo devuelve el cuerpo de Formi tal cual si es JSON; si no, lo envuelve.
func jsonCrudo(l *llamadaFormi) any {
	if json.Valid(l.Body) {
		return types.JSONRaw(l.Body)
	}
	return map[string]any{"status": l.Status, "body": string(l.Body)}
}

func rawJSON(b []byte) any {
	if json.Valid(b) {
		return types.JSONRaw(b)
	}
	return map[string]any{"body": string(b)}
}

//Correo
