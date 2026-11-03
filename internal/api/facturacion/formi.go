package facturacion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Cliente HTTP de la API de Formi Integraciones.
// Configuración por variables de entorno:
//   FORMI_API_KEY  (obligatoria) → header X-Api-Key
//   FORMI_URL      (opcional)    → por defecto https://integraciones.formi.ec

const formiURLDefault = "https://integraciones.formi.ec"

var httpClient = &http.Client{Timeout: 60 * time.Second}

// respuestaFormi es el cuerpo que devuelven POST/GET /api/facturas y el webhook.
type respuestaFormi struct {
	Ok                bool    `json:"ok"`
	FacturaID         int64   `json:"facturaId"`
	Estado            string  `json:"estado"`
	ClaveAcceso       *string `json:"claveAcceso"`
	Mensaje           *string `json:"mensaje"`
	PdfURL            *string `json:"pdfUrl"`
	XmlURL            *string `json:"xmlUrl"`
	FechaAutorizacion *string `json:"fechaAutorizacion"`
}

// llamadaFormi guarda la respuesta cruda para poder anotarla en "errores".
type llamadaFormi struct {
	Status int
	Body   []byte
}

func formiConfig() (baseURL, apiKey string, err error) {
	apiKey = strings.TrimSpace(os.Getenv("FORMI_API_KEY"))
	if apiKey == "" {
		return "", "", fmt.Errorf("falta la variable de entorno FORMI_API_KEY")
	}
	baseURL = strings.TrimRight(strings.TrimSpace(os.Getenv("FORMI_URL")), "/")
	if baseURL == "" {
		baseURL = formiURLDefault
	}
	return baseURL, apiKey, nil
}

func formiRequest(ctx context.Context, method, path string, payload any) (*llamadaFormi, error) {
	baseURL, apiKey, err := formiConfig()
	if err != nil {
		return nil, err
	}
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Api-Key", apiKey)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("no se pudo contactar a Formi: %w", err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("leyendo respuesta de Formi: %w", err)
	}
	return &llamadaFormi{Status: res.StatusCode, Body: b}, nil
}

// encolarFactura → POST /api/facturas
func encolarFactura(ctx context.Context, payload any) (*llamadaFormi, error) {
	return formiRequest(ctx, http.MethodPost, "/api/facturas", payload)
}

// consultarFactura → GET /api/facturas/{id}
func consultarFactura(ctx context.Context, formiID int64) (*llamadaFormi, error) {
	return formiRequest(ctx, http.MethodGet, fmt.Sprintf("/api/facturas/%d", formiID), nil)
}
