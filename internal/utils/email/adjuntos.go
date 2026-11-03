package email

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ses"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// Adjunto es un archivo a enviar con el correo.
type Adjunto struct {
	Nombre      string // nombre con el que lo ve el destinatario, ej. "factura.pdf"
	ContentType string // ej. "application/pdf"; si va vacío se detecta del contenido
	Datos       []byte
}

// SendEmailConAdjuntos es igual que SendEmail (misma plantilla, footer y
// remitente) pero adjunta archivos. Usa SES SendRawEmail: el mensaje completo,
// con los adjuntos en base64, no puede pasar de 10 MB.
func SendEmailConAdjuntos(app core.App, To []*string, slugPlantilla string, data map[string]string, adjuntos []Adjunto) error {
	plantilla, err := app.FindFirstRecordByFilter(
		"plantillas",
		"slug={:slug}",
		dbx.Params{"slug": slugPlantilla},
	)
	if err != nil {
		app.Logger().Error("Plantilla no encotrada para correo " + slugPlantilla)
		return fmt.Errorf("plantilla %q no encontrada: %w", slugPlantilla, err)
	}
	if !plantilla.GetBool("enviar") {
		return nil
	}
	contenido := RenderTemplate(plantilla.GetString("contenido"), data)
	asunto := RenderTemplate(plantilla.GetString("asunto"), data)
	if footer, err := app.FindFirstRecordByFilter("plantillas", "slug={:slug}", dbx.Params{"slug": "footer"}); err == nil {
		contenido += footer.GetString("contenido")
	}

	from := os.Getenv("EMAIL_ROL") + "@clinicaveterinarialoschillos.com"
	raw, err := armarMIME(from, To, asunto, contenido, adjuntos)
	if err != nil {
		return err
	}

	_, err = GetEmailClientInstance().Client.SendRawEmail(&ses.SendRawEmailInput{
		Source:       aws.String(from),
		Destinations: To,
		RawMessage:   &ses.RawMessage{Data: raw},
	})
	if err != nil {
		app.Logger().Error("Send Email (adjuntos) Error: " + err.Error())
		return err
	}
	return nil
}

// AdjuntoDesdeRecord lee un archivo guardado en un campo file de PocketBase
// (ej. factura_cabecera.pdf_file) y lo devuelve listo para adjuntar.
// Si el campo tiene varios archivos toma el primero.
func AdjuntoDesdeRecord(app core.App, record *core.Record, campo string) (Adjunto, error) {
	nombre := record.GetString(campo)
	if nombre == "" {
		if nombres := record.GetStringSlice(campo); len(nombres) > 0 {
			nombre = nombres[0]
		}
	}
	if nombre == "" {
		return Adjunto{}, fmt.Errorf("el campo %q de %s no tiene archivo", campo, record.Id)
	}

	fsys, err := app.NewFilesystem()
	if err != nil {
		return Adjunto{}, err
	}
	defer fsys.Close()

	r, err := fsys.GetReader(record.BaseFilesPath() + "/" + nombre)
	if err != nil {
		return Adjunto{}, fmt.Errorf("leyendo %s: %w", nombre, err)
	}
	defer r.Close()

	datos, err := io.ReadAll(r)
	if err != nil {
		return Adjunto{}, err
	}
	return Adjunto{Nombre: nombre, Datos: datos}, nil
}

// armarMIME construye el correo multipart/mixed: el HTML y luego cada adjunto.
func armarMIME(from string, to []*string, asunto, html string, adjuntos []Adjunto) ([]byte, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	destinatarios := make([]string, 0, len(to))
	for _, t := range to {
		if t != nil && *t != "" {
			destinatarios = append(destinatarios, *t)
		}
	}

	fmt.Fprintf(&buf, "From: %s\r\n", from)
	fmt.Fprintf(&buf, "To: %s\r\n", strings.Join(destinatarios, ", "))
	fmt.Fprintf(&buf, "Subject: %s\r\n", mime.QEncoding.Encode("UTF-8", asunto))
	fmt.Fprintf(&buf, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&buf, "Content-Type: multipart/mixed; boundary=%q\r\n\r\n", mw.Boundary())

	// Cuerpo HTML
	part, err := mw.CreatePart(textproto.MIMEHeader{
		"Content-Type":              {"text/html; charset=UTF-8"},
		"Content-Transfer-Encoding": {"base64"},
	})
	if err != nil {
		return nil, err
	}
	if err := escribirBase64(part, []byte(html)); err != nil {
		return nil, err
	}

	// Adjuntos
	for _, a := range adjuntos {
		ct := a.ContentType
		if ct == "" {
			ct = http.DetectContentType(a.Datos)
		}
		part, err := mw.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {mime.FormatMediaType(ct, map[string]string{"name": a.Nombre})},
			"Content-Disposition":       {mime.FormatMediaType("attachment", map[string]string{"filename": a.Nombre})},
			"Content-Transfer-Encoding": {"base64"},
		})
		if err != nil {
			return nil, err
		}
		if err := escribirBase64(part, a.Datos); err != nil {
			return nil, err
		}
	}

	if err := mw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// escribirBase64 escribe en base64 con líneas de 76 caracteres (RFC 2045).
func escribirBase64(w io.Writer, datos []byte) error {
	enc := base64.StdEncoding.EncodeToString(datos)
	for len(enc) > 76 {
		if _, err := io.WriteString(w, enc[:76]+"\r\n"); err != nil {
			return err
		}
		enc = enc[76:]
	}
	_, err := io.WriteString(w, enc+"\r\n")
	return err
}
