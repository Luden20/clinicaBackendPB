package facturacion

import (
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// Estados de factura_cabecera.estado
const (
	EstadoSinProcesar = "sin_procesar"
	EstadoProcesada   = "procesada"
	EstadoEnviadaSRI  = "enviada_sri"  // encolada/en proceso en Formi
	EstadoErrorSRI    = "error_sri"    // Formi o el SRI la rechazaron (detalle en "errores")
	EstadoAprobadaSRI = "aprobada_sri" // autorizada por el SRI
)

// --- Request de Formi (components/schemas/EmitirFacturaRequest) ---

type emitirFacturaRequest struct {
	NombreArchivoFirma string         `json:"nombreArchivoFirma"`
	Firma              string         `json:"firma"`
	ClaveFirma         string         `json:"claveFirma"`
	Factura            facturaRequest `json:"factura"`
}

type facturaRequest struct {
	Ambiente      string            `json:"ambiente"`
	Referencia    string            `json:"referencia,omitempty"`
	Emisor        emisor            `json:"emisor"`
	Comprador     comprador         `json:"comprador"`
	Items         []item            `json:"items"`
	Pagos         []pago            `json:"pagos"`
	EmailReceiver string            `json:"emailReceiver,omitempty"`
	InfoAdicional map[string]string `json:"informacionAdicional,omitempty"`
}

type emisor struct {
	Ruc                  string `json:"ruc"`
	RazonSocial          string `json:"razonSocial"`
	DirMatriz            string `json:"dirMatriz"`
	CodEstab             string `json:"codEstab"`
	PtoEmi               string `json:"ptoEmi"`
	Secuencial           string `json:"secuencial"`
	FechaEmision         string `json:"fechaEmision"`
	ObligadoContabilidad bool   `json:"obligadoContabilidad"`
}

type comprador struct {
	TipoIdentificacion string `json:"tipoIdentificacion"`
	Identificacion     string `json:"identificacion"`
	RazonSocial        string `json:"razonSocial"`
	Direccion          string `json:"direccion"`
	Correo             string `json:"correo,omitempty"`
}

type item struct {
	CodigoPrincipal     string  `json:"codigoPrincipal,omitempty"`
	Descripcion         string  `json:"descripcion"`
	Cantidad            float64 `json:"cantidad"`
	PrecioUnitario      float64 `json:"precioUnitario"`
	CodigoPorcentajeIva string  `json:"codigoPorcentajeIva"`
	TarifaIva           float64 `json:"tarifaIva"`
}

type pago struct {
	FormaPago string  `json:"formaPago"`
	Total     float64 `json:"total"`
}

// errorValidacion son errores de datos de la factura (→ HTTP 400, no se llama a Formi).
type errorValidacion struct{ msg string }

func (e errorValidacion) Error() string { return e.msg }

func invalida(format string, a ...any) error { return errorValidacion{fmt.Sprintf(format, a...)} }

// construirSolicitud arma el JSON para POST /api/facturas a partir de la cabecera.
func construirSolicitud(app core.App, cabecera, parametros *core.Record) (*emitirFacturaRequest, error) {
	if errs := app.ExpandRecord(cabecera, []string{"cliente", "metodos_pago", "detalles.producto"}, nil); len(errs) != 0 {
		return nil, fmt.Errorf("error al expandir la factura: %v", errs)
	}
	cliente := cabecera.ExpandedOne("cliente")
	if cliente == nil {
		return nil, invalida("la factura no tiene cliente")
	}
	detalles := cabecera.ExpandedAll("detalles")
	if len(detalles) == 0 {
		return nil, invalida("la factura no tiene detalles")
	}
	metodosPago := cabecera.ExpandedAll("metodos_pago")
	if len(metodosPago) == 0 {
		return nil, invalida("la factura no tiene métodos de pago")
	}

	ambiente, err := normalizarAmbiente(parametros.GetString("ambiente"))
	if err != nil {
		return nil, err
	}
	secuencial := parametros.GetInt("secuencial")
	if secuencial <= 0 {
		return nil, invalida("parametros_facturacion.secuencial debe ser mayor a 0")
	}

	nombreFirma, firma, err := leerFirma(app, parametros)
	if err != nil {
		return nil, err
	}

	// Comprador: cédula (05), RUC (04) o consumidor final (07).
	comp := comprador{
		Direccion: cliente.GetString("direccion"),
		Correo:    cliente.GetString("correo"),
	}
	switch {
	case cliente.GetString("cedula") != "":
		comp.TipoIdentificacion = "05"
		comp.Identificacion = cliente.GetString("cedula")
		comp.RazonSocial = cliente.GetString("nombre")
	case cliente.GetString("ruc") != "":
		comp.TipoIdentificacion = "04"
		comp.Identificacion = cliente.GetString("ruc")
		comp.RazonSocial = cliente.GetString("razon_social")
		if comp.RazonSocial == "" {
			comp.RazonSocial = cliente.GetString("nombre")
		}
	default:
		comp.TipoIdentificacion = "07"
		comp.Identificacion = "9999999999999"
		comp.RazonSocial = "CONSUMIDOR FINAL"
	}
	if comp.Direccion == "" {
		comp.Direccion = "S/N"
	}

	items := make([]item, 0, len(detalles))
	for _, detalle := range detalles {
		producto := detalle.ExpandedOne("producto")
		if producto == nil {
			return nil, invalida("el detalle %s no tiene producto", detalle.Id)
		}
		descripcion := producto.GetString("descripcion")
		if descripcion == "" {
			descripcion = producto.GetString("nombre")
		}
		codigoIva := strings.TrimSpace(producto.GetString("codigo_porcentaje_iva"))
		if codigoIva == "" {
			return nil, invalida("el producto %q no tiene codigo_porcentaje_iva", producto.GetString("nombre"))
		}
		items = append(items, item{
			CodigoPrincipal:     producto.GetString("codigo"),
			Descripcion:         descripcion,
			Cantidad:            1,
			PrecioUnitario:      detalle.GetFloat("precio"),
			CodigoPorcentajeIva: codigoIva,
			TarifaIva:           producto.GetFloat("iva"),
		})
	}

	pagos := make([]pago, 0, len(metodosPago))
	for _, mp := range metodosPago {
		codigo, ok := formasPagoSRI[mp.GetString("metodo")]
		if !ok {
			return nil, invalida("forma de pago no válida: %q", mp.GetString("metodo"))
		}
		pagos = append(pagos, pago{FormaPago: codigo, Total: mp.GetFloat("cantidad")})
	}

	return &emitirFacturaRequest{
		NombreArchivoFirma: nombreFirma,
		Firma:              firma,
		ClaveFirma:         parametros.GetString("clave_firma"),
		Factura: facturaRequest{
			Ambiente:   ambiente,
			Referencia: cabecera.Id,
			Emisor: emisor{
				Ruc:                  parametros.GetString("ruc"),
				RazonSocial:          parametros.GetString("razon_social"),
				DirMatriz:            parametros.GetString("direccion_matriz"),
				CodEstab:             fmt.Sprintf("%03d", parametros.GetInt("codigo_establecimiento")),
				PtoEmi:               fmt.Sprintf("%03d", parametros.GetInt("punto_emision")),
				Secuencial:           fmt.Sprintf("%09d", secuencial),
				FechaEmision:         time.Now().In(zonaEcuador).Format("2006-01-02"),
				ObligadoContabilidad: parametros.GetBool("obligado_contabilidad"),
			},
			Comprador:     comp,
			Items:         items,
			Pagos:         pagos,
			EmailReceiver: cliente.GetString("correo"),
		},
	}, nil
}

var formasPagoSRI = map[string]string{
	"efectivo":        "01",
	"tarjeta_debito":  "16",
	"tarjeta_credito": "19",
	"transferencia":   "20",
}

// Ecuador continental: UTC-5 sin horario de verano.
var zonaEcuador = time.FixedZone("ECT", -5*60*60)

func normalizarAmbiente(v string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "produccion", "producción":
		return "Produccion", nil
	case "test", "pruebas":
		return "Test", nil
	}
	return "", invalida("parametros_facturacion.ambiente debe ser Test o Produccion (es %q)", v)
}

func leerFirma(app core.App, parametros *core.Record) (nombre, base64Firma string, err error) {
	nombre = parametros.GetString("firma")
	if nombre == "" {
		return "", "", invalida("parametros_facturacion no tiene archivo de firma")
	}
	fsys, err := app.NewFilesystem()
	if err != nil {
		return "", "", err
	}
	defer fsys.Close()

	reader, err := fsys.GetReader(parametros.BaseFilesPath() + "/" + nombre)
	if err != nil {
		return "", "", fmt.Errorf("leyendo la firma: %w", err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return "", "", fmt.Errorf("leyendo la firma: %w", err)
	}
	return nombre, base64.StdEncoding.EncodeToString(data), nil
}
